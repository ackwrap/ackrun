package service

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"

	"golang.org/x/crypto/ssh"
)

// pkg/sftp v1.13.11 exposes extension discovery but not limits@openssh.com.
// Keep this small, bounded exchange on a dedicated subsystem; normal file I/O
// remains with pkg/sftp. Wire format: OpenSSH PROTOCOL section 4.8.
func querySFTPWriteLimit(client *ssh.Client) (int, error) {
	session, err := client.NewSession()
	if err != nil {
		return 0, err
	}
	defer session.Close()
	input, err := session.StdinPipe()
	if err != nil {
		return 0, err
	}
	output, err := session.StdoutPipe()
	if err != nil {
		return 0, err
	}
	if err := session.RequestSubsystem("sftp"); err != nil {
		return 0, err
	}
	if err := writeSFTPProbePacket(input, ssh.Marshal(struct {
		Type    byte
		Version uint32
	}{1, 3})); err != nil {
		return 0, err
	}
	version, err := readSFTPProbePacket(output)
	if err != nil {
		return 0, err
	}
	if len(version) < 5 || version[0] != 2 || binary.BigEndian.Uint32(version[1:5]) != 3 {
		return 0, errors.New("invalid SFTP version reply")
	}
	extensions := bytes.NewReader(version[5:])
	supported := false
	for extensions.Len() > 0 {
		name, err := readSFTPProbeString(extensions)
		if err != nil {
			return 0, err
		}
		value, err := readSFTPProbeString(extensions)
		if err != nil {
			return 0, err
		}
		if name == "limits@openssh.com" && value == "1" {
			supported = true
		}
	}
	if !supported {
		return sshSFTPMaxPacket, nil // Determine actual support with scratch writes.
	}
	if err := writeSFTPProbePacket(input, ssh.Marshal(struct {
		Type      byte
		ID        uint32
		Extension string
	}{200, 1, "limits@openssh.com"})); err != nil {
		return 0, err
	}
	reply, err := readSFTPProbePacket(output)
	if err != nil {
		return 0, err
	}
	var limits struct {
		Type        byte
		ID          uint32
		PacketBytes uint64
		ReadBytes   uint64
		WriteBytes  uint64
		OpenHandles uint64
	}
	if err := ssh.Unmarshal(reply, &limits); err != nil || limits.Type != 201 || limits.ID != 1 {
		return 0, errors.New("invalid SFTP limits reply")
	}
	limit := uint64(sshSFTPMaxPacket)
	if limits.WriteBytes > 0 {
		limit = min(limit, limits.WriteBytes)
	}
	if limits.PacketBytes > 0 {
		// PacketBytes includes framing/handle overhead; WriteBytes is payload.
		// Reserve room for even the maximum v3 handle and request framing.
		if limits.PacketBytes <= 1024 {
			return 0, errors.New("SFTP packet limit too small")
		}
		limit = min(limit, limits.PacketBytes-1024)
	}
	return int(limit), nil
}

func readSFTPProbeString(r *bytes.Reader) (string, error) {
	var size uint32
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		return "", err
	}
	if uint64(size) > uint64(r.Len()) {
		return "", io.ErrUnexpectedEOF
	}
	data := make([]byte, int(size))
	_, err := io.ReadFull(r, data)
	return string(data), err
}

func readSFTPProbePacket(r io.Reader) ([]byte, error) {
	var size uint32
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		return nil, err
	}
	if size < 1 || size > 64<<10 {
		return nil, errors.New("invalid SFTP probe reply size")
	}
	data := make([]byte, size)
	_, err := io.ReadFull(r, data)
	return data, err
}

func writeSFTPProbePacket(w io.Writer, data []byte) error {
	packet := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(packet, uint32(len(data)))
	copy(packet[4:], data)
	_, err := io.Copy(w, bytes.NewReader(packet))
	return err
}
