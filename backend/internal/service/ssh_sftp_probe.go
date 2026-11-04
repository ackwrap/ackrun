package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const (
	sshSFTPDefaultPacket = 32 << 10
	sshSFTPMaxPacket     = 1 << 20 // Probe downward; keep uploads within the 2 MiB window.
	sshSFTPWindowBytes   = 2 << 20
	sshSFTPProbeTimeout  = 60 * time.Second
)

type sshSFTPProbeJob struct {
	cancel context.CancelFunc
}

func (svc *SSHHostService) cancelSFTPProbe(hostID int64) {
	svc.sessionMu.Lock()
	defer svc.sessionMu.Unlock()
	if job := svc.sftpProbes[hostID]; job != nil {
		job.cancel()
		delete(svc.sftpProbes, hostID)
	}
}

// Missing profiles are also initialized once for hosts trusted before this
// feature was installed. There is no periodic expiry or per-upload negotiation.
func (svc *SSHHostService) scheduleSFTPProbe(hostID int64, previous string) {
	svc.sessionMu.Lock()
	defer svc.sessionMu.Unlock()
	if svc.closed || svc.sftpProbes[hostID] != nil {
		return
	}
	generation, err := randomHex(16)
	if err == nil {
		var started bool
		started, err = svc.store.BeginSSHSFTPProbe(hostID, generation, previous)
		if err == nil && !started {
			return
		}
	}
	if err != nil {
		logging.Error("ssh_sftp.probe", "无法记录 SFTP 探测任务: host_id=%d", hostID)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), sshSFTPProbeTimeout)
	job := &sshSFTPProbeJob{cancel: cancel}
	if svc.sftpProbes == nil {
		svc.sftpProbes = make(map[int64]*sshSFTPProbeJob)
	}
	svc.sftpProbes[hostID] = job
	svc.sftpProbeWG.Add(1)
	go func() {
		defer svc.sftpProbeWG.Done()
		defer cancel()
		defer func() {
			svc.sessionMu.Lock()
			if svc.sftpProbes[hostID] == job {
				delete(svc.sftpProbes, hostID)
			}
			svc.sessionMu.Unlock()
		}()
		logging.Info("ssh_sftp.probe", "开始 SFTP 分片探测: host_id=%d start_packet_bytes=%d", hostID, sshSFTPMaxPacket)
		size, probeErr := svc.probeSFTPPacket(ctx, hostID)
		status := "ready"
		if probeErr != nil {
			size, status = sshSFTPDefaultPacket, "fallback"
		}
		// A cancelled job must not overwrite a replacement job or finish during
		// shutdown. An interrupted 'probing' record resumes on the next use.
		if errors.Is(ctx.Err(), context.Canceled) {
			return
		}
		if err := svc.store.FinishSSHSFTPProbe(hostID, generation, size, status); err != nil {
			logging.Error("ssh_sftp.probe", "SFTP 探测结果保存失败: host_id=%d", hostID)
			return
		}
		if probeErr != nil {
			logging.Error("ssh_sftp.probe", "SFTP 探测失败，使用兼容分片: host_id=%d packet_bytes=%d", hostID, size)
		} else {
			logging.Info("ssh_sftp.probe", "SFTP 探测完成: host_id=%d packet_bytes=%d concurrency=%d", hostID, size, sshSFTPUploadConcurrency(size))
		}
	}()
}

func sshSFTPUploadConcurrency(packetSize int) int {
	return min(64, max(1, sshSFTPWindowBytes/packetSize))
}

func (svc *SSHHostService) sftpUploadProfile(hostID int64) (*model.SSHSFTPProfile, int, int, error) {
	p, err := svc.store.GetSSHSFTPProfile(hostID)
	if err != nil {
		return nil, 0, 0, err
	}
	if p == nil {
		svc.scheduleSFTPProbe(hostID, "")
	} else if p.Status == "probing" {
		svc.scheduleSFTPProbe(hostID, p.Generation)
	}
	if p == nil || p.Status != "ready" || p.PacketSize < 1 || p.PacketSize > sshSFTPMaxPacket {
		return p, sshSFTPDefaultPacket, sshMCPUploadConcurrency, nil
	}
	return p, p.PacketSize, sshSFTPUploadConcurrency(p.PacketSize), nil
}

// HTTP input errors, cancellations, full disks and permissions cannot be fixed
// by packet negotiation. Retry only transport/protocol/write failures.
func shouldReprobeSFTP(err error) bool {
	if err == nil || errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrNotExist) {
		return false
	}
	var status *sftp.StatusError
	if errors.As(err, &status) {
		return status.FxCode() == sftp.ErrSSHFxBadMessage || status.FxCode() == sftp.ErrSSHFxConnectionLost || status.FxCode() == sftp.ErrSSHFxNoConnection
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// Each candidate has its own SSH connection: a server may disconnect when an
// oversized request is received. Never run these probes on an upload session.
func (svc *SSHHostService) withSFTPProbeHost(ctx context.Context, hostID int64, run func(context.Context, *ssh.Client) error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := svc.withMCPHost(ctx, hostID, 10*time.Second, "sftp_probe", func(ctx context.Context, client *ssh.Client) error {
			if err := run(ctx, client); err != nil {
				return sshError("SSH_SFTP_PROBE_FAILED", "SFTP 分片探测请求失败", err)
			}
			return nil
		})
		var serviceErr *SSHServiceError
		if !errors.As(err, &serviceErr) || serviceErr.Code != "SSH_SESSION_LIMIT" {
			return err
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (svc *SSHHostService) probeSFTPPacket(ctx context.Context, hostID int64) (int, error) {
	limit := sshSFTPMaxPacket
	err := svc.withSFTPProbeHost(ctx, hostID, func(_ context.Context, client *ssh.Client) error {
		var err error
		limit, err = querySFTPWriteLimit(client)
		return err
	})
	if err != nil {
		return 0, err
	}
	var leftovers []string
	defer func() {
		if len(leftovers) == 0 {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// A rejected packet can kill its SFTP session. Clean up using a new,
		// default-sized session rather than leaving probe files on the host.
		err := svc.withSFTPProbeHost(cleanupCtx, hostID, func(_ context.Context, client *ssh.Client) error {
			files, err := sftp.NewClient(client)
			if err != nil {
				return err
			}
			defer files.Close()
			for _, name := range leftovers {
				if err := files.Remove(name); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
			return nil
		})
		if err != nil {
			logging.Error("ssh_sftp.probe_cleanup", "SFTP 探测文件清理失败: host_id=%d", hostID)
		}
	}()
	try := func(size int) error {
		return svc.withSFTPProbeHost(ctx, hostID, func(_ context.Context, client *ssh.Client) error {
			files, err := sftp.NewClient(client, sftp.MaxPacketUnchecked(size))
			if err != nil {
				return err
			}
			defer files.Close()
			suffix, err := randomHex(16)
			if err != nil {
				return err
			}
			// Relative to the SFTP login directory; works with chroot and Windows
			// servers too, without invoking a shell or assuming /tmp exists.
			name := ".ackwrap-sftp-probe-" + suffix
			file, err := files.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL)
			if err != nil {
				return err
			}
			defer func() {
				_ = file.Close()
				if err := files.Remove(name); err != nil {
					leftovers = append(leftovers, name)
				}
			}()
			if err := file.Chmod(0o600); err != nil {
				return err
			}
			payload := make([]byte, size)
			for i := range payload {
				payload[i] = byte(i % 251)
			}
			if n, err := file.Write(payload); err != nil || n != size {
				return errors.Join(err, io.ErrShortWrite)
			}
			// Read back in compatible chunks; the detected value is a WRITE limit,
			// not permission to exceed a server's independent READ limit.
			for offset := 0; offset < size; offset += sshSFTPDefaultPacket {
				want := payload[offset:min(size, offset+sshSFTPDefaultPacket)]
				got := make([]byte, len(want))
				if n, err := file.ReadAt(got, int64(offset)); err != nil || n != len(want) || !bytes.Equal(got, want) {
					return fmt.Errorf("SFTP probe readback failed: %w", errors.Join(err, io.ErrUnexpectedEOF))
				}
			}
			return file.Close()
		})
	}
	return probeSFTPWriteSize(ctx, limit, try)
}

func probeSFTPWriteSize(ctx context.Context, advertisedLimit int, try func(int) error) (int, error) {
	good, bad := 0, sshSFTPMaxPacket+1
	floor := min(sshSFTPDefaultPacket, advertisedLimit)
	check := func(size int) (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		err := try(size)
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		var serviceErr *SSHServiceError
		if errors.As(err, &serviceErr) && serviceErr.Code != "SSH_SFTP_PROBE_FAILED" {
			return false, err
		}
		// An oversized WRITE can return generic SSH_FX_FAILURE rather than
		// closing the channel. It still needs a smaller candidate; upload-error
		// retry classification cannot distinguish that probe response.
		return err == nil, nil
	}
	// Always verify 1 MiB first, then halve on independent connections until a
	// candidate succeeds within the advertised limit. The advertised limit must
	// not skip the initial 1 MiB WRITE or persist an unverified packet size.
	for candidate := sshSFTPMaxPacket; ; candidate = max(floor, candidate/2) {
		ok, err := check(candidate)
		if err != nil {
			return 0, err
		}
		if ok && candidate <= advertisedLimit {
			good = candidate
			break
		}
		if candidate == floor {
			return 0, errors.New("SFTP rejected the compatible probe size")
		}
		if !ok {
			bad = candidate
		}
	}
	bad = min(bad, advertisedLimit+1)
	// Confirm a useful advertised boundary directly (for example 255 KiB).
	// Servers without this extension still use the measured success/rejection.
	if advertisedLimit > good && advertisedLimit < bad {
		ok, err := check(advertisedLimit)
		if err != nil {
			return 0, err
		}
		bad = advertisedLimit
		if ok {
			good, bad = advertisedLimit, advertisedLimit+1
		}
	}
	// Refine between the last success and first rejection to 4 KiB. Do not
	// claim an untested protocol maximum or a throughput benchmark.
	for bad-good > 4<<10 {
		candidate := (good + bad) / 2
		ok, err := check(candidate)
		if err != nil {
			return 0, err
		}
		if !ok {
			bad = candidate
		} else {
			good = candidate
		}
	}
	return good, ctx.Err()
}
