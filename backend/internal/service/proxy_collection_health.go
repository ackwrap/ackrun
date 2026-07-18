package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/robfig/cron/v3"
)

func (s *ProxyCollectionService) StartScheduler() {
	s.healthJobsMu.Lock()
	defer s.healthJobsMu.Unlock()

	collections, err := s.store.ListProxyCollections()
	if err != nil {
		logging.Error("proxy_collection.scheduler", "加载策略组失败: %v", err)
		return
	}
	for _, collection := range collections {
		s.scheduleHealthCheckJobLocked(collection)
	}
	s.cron.Start()
	s.mu.Lock()
	taskCount := len(s.entries)
	s.mu.Unlock()
	logging.Info("proxy_collection.scheduler", "健康检查调度器已启动，任务数: %d", taskCount)
}

func (s *ProxyCollectionService) StopScheduler() {
	s.healthJobsMu.Lock()
	defer s.healthJobsMu.Unlock()

	ctx := s.cron.Stop()
	<-ctx.Done()
	logging.Info("proxy_collection.scheduler", "健康检查调度器已停止")
}

func (s *ProxyCollectionService) scheduleHealthCheckJobLocked(collection *model.ProxyCollection) {
	if collection == nil || !collection.Enabled || collection.Type != "urltest" {
		return
	}
	settings, err := s.store.GetConnectivitySettings()
	if err != nil {
		logging.Error("proxy_collection.scheduler", "读取连通性测速设置失败 collection=%d: %v", collection.ID, err)
		return
	}
	entryID, err := s.cron.AddFunc(fmt.Sprintf("@every %ds", settings.IntervalSeconds), func() {
		if _, err := s.Test(collection.ID); err != nil {
			logging.Error("proxy_collection.test", "定时健康检查失败 collection=%d: %v", collection.ID, err)
			if s.realtime != nil {
				s.realtime.Broadcast("collection.test", &model.CollectionTestResponse{CollectionID: collection.ID, Error: err.Error(), Results: []model.CollectionTestNodeResult{}})
			}
		}
	})
	if err != nil {
		logging.Error("proxy_collection.scheduler", "添加健康检查任务失败 collection=%d: %v", collection.ID, err)
		return
	}
	s.mu.Lock()
	s.entries[collection.ID] = entryID
	s.mu.Unlock()
}

func (s *ProxyCollectionService) RefreshHealthCheckJobs() {
	s.healthJobsMu.Lock()
	defer s.healthJobsMu.Unlock()

	s.mu.Lock()
	entryIDs := make([]cron.EntryID, 0, len(s.entries))
	for _, entryID := range s.entries {
		entryIDs = append(entryIDs, entryID)
	}
	s.entries = make(map[int]cron.EntryID)
	s.mu.Unlock()
	for _, entryID := range entryIDs {
		s.cron.Remove(entryID)
	}

	collections, err := s.store.ListProxyCollections()
	if err != nil {
		logging.Error("proxy_collection.scheduler", "刷新健康检查任务失败: %v", err)
		return
	}
	for _, collection := range collections {
		s.scheduleHealthCheckJobLocked(collection)
	}
	s.mu.Lock()
	taskCount := len(s.entries)
	s.mu.Unlock()
	logging.Info("proxy_collection.scheduler", "健康检查任务已刷新，任务数: %d", taskCount)
}

func (s *ProxyCollectionService) removeHealthCheckJob(id int) {
	s.healthJobsMu.Lock()
	defer s.healthJobsMu.Unlock()
	s.removeHealthCheckJobLocked(id)
}

func (s *ProxyCollectionService) removeHealthCheckJobLocked(id int) {
	s.mu.Lock()
	entryID, ok := s.entries[id]
	if ok {
		delete(s.entries, id)
	}
	s.mu.Unlock()
	if ok {
		s.cron.Remove(entryID)
	}
}

func (s *ProxyCollectionService) refreshHealthCheckJob(id int) {
	s.healthJobsMu.Lock()
	defer s.healthJobsMu.Unlock()

	s.removeHealthCheckJobLocked(id)
	collection, err := s.store.GetProxyCollection(id)
	if err == nil {
		s.scheduleHealthCheckJobLocked(collection)
	}
}

func (s *ProxyCollectionService) Test(id int) (*model.CollectionTestResponse, error) {
	s.mu.Lock()
	if s.runningTests[id] {
		s.mu.Unlock()
		return nil, fmt.Errorf("该策略组正在测速")
	}
	s.runningTests[id] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.runningTests, id)
		s.mu.Unlock()
	}()

	collection, err := s.store.GetProxyCollectionWithNodes(id)
	if err != nil {
		return nil, err
	}
	nodes, err := s.collectionNodes(collection)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("策略组没有可测速的已启用节点")
	}
	settings, err := s.store.GetConnectivitySettings()
	if err != nil {
		return nil, fmt.Errorf("读取连通性测速设置失败: %w", err)
	}

	logging.Info("proxy_collection.test", "开始策略组健康检查 collection=%d nodes=%d", id, len(nodes))
	results := make([]model.CollectionTestNodeResult, 0, len(nodes))
	testedAt := time.Now().UnixMilli()
	nodeTags := buildNodeOutboundTags(nodes)
	jobs := make(chan model.Node)
	resultCh := make(chan model.CollectionTestNodeResult, len(nodes))
	workerCount := min(len(nodes), 8)
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for node := range jobs {
				resultCh <- s.testNode(node.UID, nodeTags[node.UID], settings.TestURL)
			}
		}()
	}
	go func() {
		for _, node := range nodes {
			jobs <- node
		}
		close(jobs)
		workers.Wait()
		close(resultCh)
	}()
	for result := range resultCh {
		results = append(results, result)
		if err := s.store.UpdateNodeHealthCheck(result.UID, result.LatencyMS, result.Success, testedAt); err != nil {
			logging.Error("proxy_collection.test", "保存健康检查结果失败 uid=%s: %v", result.UID, err)
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Success != results[j].Success {
			return results[i].Success
		}
		return results[i].LatencyMS < results[j].LatencyMS
	})
	response := &model.CollectionTestResponse{CollectionID: id, Tested: len(results), Results: results}
	for _, result := range results {
		if result.Success {
			response.Available++
			if response.FastestUID == "" {
				response.FastestUID = result.UID
				response.FastestLatency = result.LatencyMS
			}
		} else if response.Error == "" {
			response.Error = result.Error
		}
	}
	if response.Available > 0 {
		response.Error = ""
	}
	if s.realtime != nil {
		s.realtime.Broadcast("collection.test", response)
	}
	logging.Info("proxy_collection.test", "策略组健康检查完成 collection=%d tested=%d available=%d", id, response.Tested, response.Available)
	return response, nil
}

func (s *ProxyCollectionService) collectionNodes(collection *model.ProxyCollectionWithNodes) ([]model.Node, error) {
	seen := make(map[string]bool)
	nodes := make([]model.Node, 0)
	appendNodes := func(items []model.Node) {
		for _, node := range items {
			if node.Enabled && node.UID != "" && !seen[node.UID] {
				seen[node.UID] = true
				nodes = append(nodes, node)
			}
		}
	}
	if !isCollectionGroupSource(collection.SourceType) {
		items, err := s.store.ListNodesByUIDs(collection.NodeUIDs)
		if err != nil {
			return nil, err
		}
		appendNodes(items)
		return nodes, nil
	}
	for _, group := range collection.ReferencedGroups {
		var items []model.Node
		var err error
		if strings.TrimSpace(group.NodeUIDs) != "" && strings.TrimSpace(group.NodeUIDs) != "[]" {
			items, err = s.store.PreviewNodeGroupManualMatches(group.NodeUIDs)
		} else {
			items, err = s.store.PreviewNodeGroupMatches(group.FilterProtocols, group.FilterSubscriptions, group.FilterInclude, group.FilterExclude)
		}
		if err != nil {
			return nil, err
		}
		appendNodes(items)
	}
	return nodes, nil
}

func (s *ProxyCollectionService) testNode(uid, outboundTag, testURL string) model.CollectionTestNodeResult {
	baseURL, secret, err := s.clashAPI()
	if err != nil {
		return model.CollectionTestNodeResult{UID: uid, Error: err.Error()}
	}
	endpoint := fmt.Sprintf("%s/proxies/%s/delay?timeout=10000&url=%s", baseURL, url.PathEscape(outboundTag), url.QueryEscape(testURL))
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return model.CollectionTestNodeResult{UID: uid, Error: err.Error()}
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return model.CollectionTestNodeResult{UID: uid, Error: "无法连接 sing-box Clash API: " + err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return model.CollectionTestNodeResult{UID: uid, Error: "节点尚未载入当前运行配置，请等待配置自动应用完成后重试"}
		}
		return model.CollectionTestNodeResult{UID: uid, Error: fmt.Sprintf("Clash API 返回 HTTP %d", resp.StatusCode)}
	}
	var payload struct {
		Delay int `json:"delay"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return model.CollectionTestNodeResult{UID: uid, Error: "无法解析测速响应: " + err.Error()}
	}
	if payload.Delay <= 0 {
		return model.CollectionTestNodeResult{UID: uid, Error: "测速未返回有效延迟"}
	}
	return model.CollectionTestNodeResult{UID: uid, Success: true, LatencyMS: payload.Delay}
}

func (s *ProxyCollectionService) clashAPI() (string, string, error) {
	if s.clashBaseURL != "" {
		return strings.TrimRight(s.clashBaseURL, "/"), "", nil
	}
	settings, err := s.store.GetExperimentalSettings()
	if err != nil {
		return "", "", err
	}
	if settings == nil || settings.ClashAPIPort == "" {
		return "", "", fmt.Errorf("Clash API 未配置")
	}
	return "http://127.0.0.1:" + settings.ClashAPIPort, settings.ClashAPISecret, nil
}
