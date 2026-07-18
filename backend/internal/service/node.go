package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/parser"
	"github.com/ackwrap/ackwrap/internal/store"
	"github.com/ackwrap/ackwrap/internal/traceroute"
)

var ErrNodeNotFound = errors.New("node not found")
var ErrTracerouteInvalid = errors.New("invalid traceroute request")

var tracerouteIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

type nodeTracerouteTask struct {
	uid    string
	cancel context.CancelFunc
}

type NodeService struct {
	store        *store.Store
	clashBaseURL string
	httpClient   *http.Client
	realtime     *RealtimeService
	traceMu      sync.Mutex
	traces       map[string]nodeTracerouteTask
}

func NewNodeService(s *store.Store) *NodeService {
	return &NodeService{store: s, traces: make(map[string]nodeTracerouteTask)}
}

func (svc *NodeService) SetRealtimeService(realtime *RealtimeService) {
	svc.realtime = realtime
}

func (svc *NodeService) List(req model.NodeListRequest) (*model.NodeListResponse, error) {
	req.Keyword = strings.TrimSpace(req.Keyword)
	req.Type = strings.TrimSpace(req.Type)
	req.Status = strings.TrimSpace(req.Status)
	logging.Info("node.list", "listing nodes subscription_id=%d keyword=%s type=%s status=%s", req.SubscriptionID, req.Keyword, req.Type, req.Status)
	return svc.store.ListNodes(req)
}

func (svc *NodeService) Facets() (*model.NodeFacetsResponse, error) {
	logging.Info("node.facets", "loading node facets")
	return svc.store.NodeFacets()
}

func (svc *NodeService) Import(req model.NodeImportRequest) (*model.NodeImportResponse, error) {
	nodes, err := svc.parseImportNodes(req.Content)
	if err != nil {
		return nil, err
	}
	manual, err := svc.store.EnsureManualSubscription()
	if err != nil {
		return nil, err
	}
	if err := svc.store.UpsertSubscriptionNodes(manual.ID, nodes); err != nil {
		return nil, err
	}
	logging.Info("node.import", "imported %d nodes into manual subscription %d", len(nodes), manual.ID)
	return &model.NodeImportResponse{Imported: len(nodes), SubscriptionID: manual.ID}, nil
}

func (svc *NodeService) ImportPreview(req model.NodeImportRequest) (*model.NodeImportPreviewResponse, error) {
	nodes, err := svc.parseImportNodes(req.Content)
	if err != nil {
		return nil, err
	}
	items := make([]model.NodeImportPreviewItem, 0, len(nodes))
	for _, node := range nodes {
		uid := node.UID
		if uid == "" {
			uid = store.StableNodeUID(node)
		}
		items = append(items, model.NodeImportPreviewItem{Name: node.Name, Type: node.Type, Server: node.Server, ServerPort: node.ServerPort, UID: uid, RawJSON: node.RawJSON})
	}
	return &model.NodeImportPreviewResponse{Count: len(items), Items: items}, nil
}

func (svc *NodeService) parseImportNodes(content string) ([]model.ParsedNode, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("import content is required")
	}
	nodes, err := parser.ParseSubscriptionNodes([]byte(content))
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("import content contains no supported nodes")
	}

	// 过滤不支持的协议和无法安全等价转换的 Clash 协议变体。
	supportedNodes := make([]model.ParsedNode, 0, len(nodes))
	unsupportedCount := 0

	for _, node := range nodes {
		if isUnsupportedNodeType(node.Type) || node.UnsupportedReason != "" {
			unsupportedCount++
			if node.UnsupportedReason != "" {
				logging.Info("node.import", "filtered %s node: %s", node.Type, node.UnsupportedReason)
			}
		} else {
			supportedNodes = append(supportedNodes, node)
		}
	}

	if unsupportedCount > 0 {
		logging.Info("node.import", "filtered %d unsupported nodes", unsupportedCount)
	}

	if len(supportedNodes) == 0 {
		return nil, fmt.Errorf("all nodes use unsupported protocols or options")
	}

	return svc.applyNodeFilters(supportedNodes)
}

func (svc *NodeService) applyNodeFilters(nodes []model.ParsedNode) ([]model.ParsedNode, error) {
	filters, err := svc.store.ListEnabledNodeFilters()
	if err != nil {
		return nil, err
	}
	if len(filters) == 0 {
		return nodes, nil
	}
	compiled := make([]compiledImportNodeFilter, 0, len(filters))
	for _, filter := range filters {
		re, err := regexp.Compile(filter.Pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid node filter %s: %w", filter.Name, err)
		}
		compiled = append(compiled, compiledImportNodeFilter{filter: filter, regex: re})
	}
	kept := make([]model.ParsedNode, 0, len(nodes))
	for _, node := range nodes {
		if importNodeFiltered(node, compiled) {
			continue
		}
		kept = append(kept, node)
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("all imported nodes were filtered by rules")
	}
	return kept, nil
}

type compiledImportNodeFilter struct {
	filter model.NodeFilter
	regex  *regexp.Regexp
}

func importNodeFiltered(node model.ParsedNode, filters []compiledImportNodeFilter) bool {
	for _, filter := range filters {
		if filter.regex.MatchString(importNodeFilterValue(node, filter.filter.Target)) {
			return true
		}
	}
	return false
}

func importNodeFilterValue(node model.ParsedNode, target string) string {
	switch target {
	case "name":
		return node.Name
	case "type":
		return node.Type
	case "server":
		return node.Server
	case "raw":
		return node.Raw
	case "raw_json":
		return node.RawJSON
	case "all":
		fallthrough
	default:
		return strings.Join([]string{node.Name, node.Type, node.Server, node.Raw, node.RawJSON}, "\n")
	}
}

func (svc *NodeService) SetEnabled(uid string, enabled bool) (*model.ActionResponse, error) {
	logging.Info("node.enabled", "setting node %s enabled=%v", uid, enabled)
	if err := svc.store.SetNodeEnabled(uid, enabled); err != nil {
		return nil, err
	}
	return &model.ActionResponse{Success: true, Message: "node enabled updated"}, nil
}

func (svc *NodeService) SetPreferred(uid string, preferred bool) (*model.ActionResponse, error) {
	logging.Info("node.preferred", "setting node %s preferred=%v", uid, preferred)
	if err := svc.store.SetNodePreferred(uid, preferred); err != nil {
		return nil, err
	}
	return &model.ActionResponse{Success: true, Message: "node preferred updated"}, nil
}

func (svc *NodeService) TCPing(uids []string) ([]model.NodeTCPingResult, error) {
	nodes, err := svc.store.ListNodesByUIDs(uids)
	if err != nil {
		return nil, err
	}
	results := make([]model.NodeTCPingResult, 0, len(nodes))
	nodeTags := buildNodeOutboundTags(nodes)
	for _, node := range nodes {
		var result model.NodeTCPingResult
		if usesUDPTransport(node.Type) {
			result = svc.urlTestNode(node, nodeTags[node.UID])
		} else {
			result = svc.tcpingNode(node)
		}
		results = append(results, result)
		status := "unavailable"
		latency := 0
		if result.Success {
			status = "available"
			latency = result.LatencyMS
		}
		if err := svc.store.UpdateNodeTCPing(node.UID, latency, status); err != nil {
			logging.Error("node.tcping", "update tcping result failed for %s: %v", node.UID, err)
		}
	}
	return results, nil
}

func (svc *NodeService) StartTraceroute(uid string, traceID string, geoProvider string) (*model.NodeTracerouteStartResponse, error) {
	traceID = strings.TrimSpace(traceID)
	if !tracerouteIDPattern.MatchString(traceID) {
		return nil, fmt.Errorf("%w: invalid trace id", ErrTracerouteInvalid)
	}
	geoProvider, err := traceroute.ValidateGeoProvider(geoProvider)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTracerouteInvalid, err)
	}
	nodes, err := svc.store.ListNodesByUIDs([]string{uid})
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, ErrNodeNotFound
	}
	if svc.realtime == nil {
		return nil, fmt.Errorf("realtime service is unavailable")
	}
	node := nodes[0]
	ctx, cancel := context.WithCancel(context.Background())
	svc.traceMu.Lock()
	if _, exists := svc.traces[traceID]; exists {
		svc.traceMu.Unlock()
		cancel()
		return nil, fmt.Errorf("%w: trace id is already running", ErrTracerouteInvalid)
	}
	svc.traces[traceID] = nodeTracerouteTask{uid: uid, cancel: cancel}
	svc.traceMu.Unlock()

	logging.Info("node.traceroute", "starting trace uid=%s trace_id=%s geo_provider=%s", uid, traceID, geoProvider)
	svc.broadcastTraceroute(model.NodeTracerouteEvent{
		TraceID:     traceID,
		UID:         uid,
		NodeName:    node.Name,
		Status:      "started",
		Target:      node.Server,
		Protocol:    "ICMP",
		GeoProvider: geoProvider,
	})
	go svc.runTraceroute(ctx, traceID, node, geoProvider)
	return &model.NodeTracerouteStartResponse{TraceID: traceID, UID: uid, Status: "started", GeoProvider: geoProvider}, nil
}

func (svc *NodeService) runTraceroute(ctx context.Context, traceID string, node model.Node, geoProvider string) {
	started := time.Now()
	defer func() {
		svc.traceMu.Lock()
		delete(svc.traces, traceID)
		svc.traceMu.Unlock()
	}()

	options := traceroute.DefaultOptions()
	options.GeoProvider = geoProvider
	result, err := traceroute.TraceWithProgress(ctx, node.Server, options, func(partial traceroute.Result, hop traceroute.Hop) {
		converted := convertTracerouteHop(hop)
		svc.broadcastTraceroute(model.NodeTracerouteEvent{
			TraceID:     traceID,
			UID:         node.UID,
			NodeName:    node.Name,
			Status:      "hop",
			Target:      node.Server,
			ResolvedIP:  partial.ResolvedIP,
			Protocol:    partial.Protocol,
			IPVersion:   partial.IPVersion,
			Reached:     partial.Reached,
			DurationMS:  time.Since(started).Milliseconds(),
			GeoProvider: partial.GeoProvider,
			Hop:         &converted,
		})
	})
	if err != nil {
		status := "failed"
		message := err.Error()
		if errors.Is(err, context.Canceled) {
			status = "canceled"
			message = ""
			logging.Info("node.traceroute", "trace canceled uid=%s trace_id=%s", node.UID, traceID)
		} else {
			logging.Error("node.traceroute", "trace failed uid=%s trace_id=%s: %v", node.UID, traceID, err)
		}
		svc.broadcastTraceroute(model.NodeTracerouteEvent{
			TraceID: traceID, UID: node.UID, NodeName: node.Name, Status: status,
			Target: node.Server, DurationMS: time.Since(started).Milliseconds(), GeoProvider: geoProvider, Error: message,
		})
		return
	}
	svc.broadcastTraceroute(model.NodeTracerouteEvent{
		TraceID:     traceID,
		UID:         node.UID,
		NodeName:    node.Name,
		Status:      "completed",
		Target:      node.Server,
		ResolvedIP:  result.ResolvedIP,
		Protocol:    result.Protocol,
		IPVersion:   result.IPVersion,
		Reached:     result.Reached,
		DurationMS:  result.Duration.Milliseconds(),
		GeoProvider: result.GeoProvider,
	})
	logging.Info("node.traceroute", "trace completed uid=%s trace_id=%s hops=%d reached=%v", node.UID, traceID, len(result.Hops), result.Reached)
}

func (svc *NodeService) CancelTraceroute(uid string, traceID string) (*model.ActionResponse, error) {
	svc.traceMu.Lock()
	task, exists := svc.traces[traceID]
	svc.traceMu.Unlock()
	if !exists || task.uid != uid {
		return nil, fmt.Errorf("traceroute task not found")
	}
	task.cancel()
	return &model.ActionResponse{Success: true, Message: "traceroute canceled"}, nil
}

func (svc *NodeService) broadcastTraceroute(event model.NodeTracerouteEvent) {
	if svc.realtime != nil {
		svc.realtime.Broadcast("node.traceroute", event)
	}
}

func convertTracerouteHop(hop traceroute.Hop) model.NodeTracerouteHop {
	attempts := make([]model.NodeTracerouteAttempt, len(hop.Attempts))
	for index, attempt := range hop.Attempts {
		var geo *model.NodeTracerouteGeo
		if attempt.Success {
			geo = convertTracerouteGeo(attempt.Geo)
		}
		attempts[index] = model.NodeTracerouteAttempt{
			Success:  attempt.Success,
			IP:       attempt.IP,
			Hostname: attempt.Hostname,
			RTTMS:    math.Round(float64(attempt.RTT)/float64(time.Millisecond)*100) / 100,
			Reached:  attempt.Reached,
			Geo:      geo,
			GeoError: attempt.GeoError,
		}
	}
	return model.NodeTracerouteHop{TTL: hop.TTL, Attempts: attempts}
}

func convertTracerouteGeo(geo traceroute.GeoData) *model.NodeTracerouteGeo {
	return &model.NodeTracerouteGeo{
		ASN:        geo.ASN,
		Country:    geo.Country,
		CountryEn:  geo.CountryEn,
		Province:   geo.Province,
		ProvinceEn: geo.ProvinceEn,
		City:       geo.City,
		CityEn:     geo.CityEn,
		District:   geo.District,
		Owner:      geo.Owner,
		ISP:        geo.ISP,
		Domain:     geo.Domain,
		Whois:      geo.Whois,
		Latitude:   geo.Latitude,
		Longitude:  geo.Longitude,
		Prefix:     geo.Prefix,
		Source:     geo.Source,
	}
}

func usesUDPTransport(nodeType string) bool {
	switch strings.ToLower(strings.TrimSpace(nodeType)) {
	case "hysteria", "hysteria2", "tuic", "wireguard":
		return true
	default:
		return false
	}
}

func (svc *NodeService) urlTestNode(node model.Node, outboundTag string) model.NodeTCPingResult {
	baseURL, secret, err := svc.nodeClashAPI()
	if err != nil {
		return model.NodeTCPingResult{UID: node.UID, Error: err.Error()}
	}
	endpoint := fmt.Sprintf(
		"%s/proxies/%s/delay?timeout=10000&url=%s",
		baseURL,
		url.PathEscape(outboundTag),
		url.QueryEscape("https://www.gstatic.com/generate_204"),
	)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return model.NodeTCPingResult{UID: node.UID, Error: err.Error()}
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	client := svc.httpClient
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return model.NodeTCPingResult{UID: node.UID, Error: "无法连接 sing-box Clash API: " + err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return model.NodeTCPingResult{UID: node.UID, Error: fmt.Sprintf("sing-box 协议测速返回 HTTP %d，节点可能未载入当前配置", resp.StatusCode)}
	}
	var payload struct {
		Delay int `json:"delay"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return model.NodeTCPingResult{UID: node.UID, Error: "无法解析 sing-box 协议测速响应: " + err.Error()}
	}
	if payload.Delay <= 0 {
		return model.NodeTCPingResult{UID: node.UID, Error: "sing-box 协议测速未返回有效延迟"}
	}
	return model.NodeTCPingResult{UID: node.UID, Success: true, LatencyMS: payload.Delay}
}

func (svc *NodeService) nodeClashAPI() (string, string, error) {
	if svc.clashBaseURL != "" {
		return strings.TrimRight(svc.clashBaseURL, "/"), "", nil
	}
	settings, err := svc.store.GetExperimentalSettings()
	if err != nil {
		return "", "", fmt.Errorf("读取 Clash API 设置失败: %w", err)
	}
	port := "9090"
	secret := ""
	if settings != nil {
		if settings.ClashAPIPort != "" {
			port = settings.ClashAPIPort
		}
		secret = settings.ClashAPISecret
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", "", fmt.Errorf("Clash API 端口无效")
	}
	return "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(portNumber)), secret, nil
}

func (svc *NodeService) tcpingNode(node model.Node) model.NodeTCPingResult {
	start := time.Now()
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.Dial("tcp", net.JoinHostPort(node.Server, fmt.Sprintf("%d", node.ServerPort)))
	if err != nil {
		return model.NodeTCPingResult{UID: node.UID, Success: false, Error: err.Error()}
	}
	_ = conn.Close()
	return model.NodeTCPingResult{UID: node.UID, Success: true, LatencyMS: int(time.Since(start).Milliseconds())}
}

func (svc *NodeService) AddEmoji(uids []string) (*model.NodeBatchResult, error) {
	nodes, err := svc.store.ListNodesByUIDs(uids)
	if err != nil {
		return nil, err
	}
	result := &model.NodeBatchResult{}
	for _, node := range nodes {
		emoji := inferNodeEmoji(node)
		if emoji == "" || strings.HasPrefix(node.Name, emoji) {
			result.Failed++
			continue
		}
		if err := svc.store.UpdateNodeName(node.UID, emoji+" "+node.Name); err != nil {
			result.Failed++
			continue
		}
		result.Success++
	}
	return result, nil
}

func (svc *NodeService) InferFlag(req model.NodeFlagRequest) model.NodeFlagResponse {
	flag := inferNodeEmoji(model.Node{Name: req.Name, Server: req.Server})
	if flag == "" {
		flag = "🇺🇳"
	}
	return model.NodeFlagResponse{Flag: flag}
}

func (svc *NodeService) InferFlags(req model.NodeFlagBatchRequest) model.NodeFlagBatchResponse {
	items := make([]model.NodeFlagBatchResult, 0, len(req.Items))
	for _, item := range req.Items {
		flag := inferNodeEmoji(model.Node{Name: item.Name, Server: item.Server})
		if flag == "" {
			flag = "🇺🇳"
		}
		items = append(items, model.NodeFlagBatchResult{Key: item.Key, Flag: flag})
	}
	return model.NodeFlagBatchResponse{Items: items}
}

func (svc *NodeService) BatchRename(req model.NodeBatchRenameRequest) (*model.NodeBatchResult, error) {
	nodes, err := svc.store.ListNodesByUIDs(req.UIDs)
	if err != nil {
		return nil, err
	}
	result := &model.NodeBatchResult{}
	for i, node := range nodes {
		name, ok := nextNodeName(node.Name, i, req)
		if !ok || strings.TrimSpace(name) == "" {
			result.Failed++
			continue
		}
		if err := svc.store.UpdateNodeName(node.UID, strings.TrimSpace(name)); err != nil {
			result.Failed++
			continue
		}
		result.Success++
	}
	return result, nil
}

func (svc *NodeService) BatchDelete(uids []string) (*model.NodeBatchResult, error) {
	logging.Info("node.delete", "deleting %d nodes", len(uids))
	result := &model.NodeBatchResult{}
	for _, uid := range uids {
		if err := svc.store.DeleteNode(uid); err != nil {
			logging.Error("node.delete", "failed to delete node uid=%s: %v", uid, err)
			result.Failed++
		} else {
			result.Success++
		}
	}
	return result, nil
}

func nextNodeName(current string, index int, req model.NodeBatchRenameRequest) (string, bool) {
	switch req.Mode {
	case "lines":
		if index >= len(req.Names) {
			return "", false
		}
		return req.Names[index], true
	case "replace":
		return strings.ReplaceAll(current, req.Find, req.Replace), req.Find != ""
	case "prefix":
		return req.Prefix + current, req.Prefix != ""
	case "suffix":
		return current + req.Suffix, req.Suffix != ""
	default:
		return "", false
	}
}

func inferNodeEmoji(node model.Node) string {
	text := strings.ToLower(node.Name + " " + node.Server)
	rules := []struct {
		keys  []string
		emoji string
	}{
		{[]string{"hongkong", "hong kong", "hk", "香港", "港"}, "🇭🇰"},
		{[]string{"taiwan", "taipei", "tw", "台湾", "台灣", "台北"}, "🇹🇼"},
		{[]string{"japan", "jp", "tokyo", "osaka", "日本", "东京", "東京", "大阪"}, "🇯🇵"},
		{[]string{"singapore", "sg", "新加坡", "狮城", "獅城"}, "🇸🇬"},
		{[]string{"united states", "usa", "us", "america", "los angeles", "san jose", "seattle", "美国", "美國", "洛杉矶", "洛杉磯", "圣何塞", "聖何塞", "西雅图", "西雅圖"}, "🇺🇸"},
		{[]string{"korea", "kr", "seoul", "韩国", "韓國", "首尔", "首爾"}, "🇰🇷"},
		{[]string{"uk", "gb", "united kingdom", "britain", "london", "英国", "英國", "伦敦", "倫敦"}, "🇬🇧"},
		{[]string{"germany", "de", "frankfurt", "德国", "德國", "法兰克福", "法蘭克福"}, "🇩🇪"},
		{[]string{"france", "fr", "paris", "法国", "法國", "巴黎"}, "🇫🇷"},
		{[]string{"netherlands", "nl", "amsterdam", "荷兰", "荷蘭", "阿姆斯特丹"}, "🇳🇱"},
		{[]string{"canada", "ca", "toronto", "vancouver", "加拿大", "多伦多", "多倫多", "温哥华", "溫哥華"}, "🇨🇦"},
		{[]string{"australia", "au", "sydney", "澳大利亚", "澳大利亞", "澳洲", "悉尼"}, "🇦🇺"},
		{[]string{"india", "in", "mumbai", "印度", "孟买", "孟買"}, "🇮🇳"},
		{[]string{"thailand", "th", "bangkok", "泰国", "泰國", "曼谷"}, "🇹🇭"},
		{[]string{"vietnam", "vn", "hochiminh", "ho chi minh", "越南", "胡志明"}, "🇻🇳"},
		{[]string{"philippines", "ph", "manila", "菲律宾", "菲律賓", "马尼拉", "馬尼拉"}, "🇵🇭"},
		{[]string{"malaysia", "my", "kuala lumpur", "马来西亚", "馬來西亞", "吉隆坡"}, "🇲🇾"},
		{[]string{"indonesia", "id", "jakarta", "印度尼西亚", "印度尼西亞", "印尼", "雅加达", "雅加達"}, "🇮🇩"},
		{[]string{"russia", "ru", "moscow", "俄罗斯", "俄羅斯", "莫斯科"}, "🇷🇺"},
		{[]string{"turkey", "tr", "istanbul", "土耳其", "伊斯坦布尔", "伊斯坦堡"}, "🇹🇷"},
		{[]string{"brazil", "br", "sao paulo", "巴西", "圣保罗", "聖保羅"}, "🇧🇷"},
		{[]string{"argentina", "ar", "阿根廷"}, "🇦🇷"},
		{[]string{"mexico", "mx", "墨西哥"}, "🇲🇽"},
		{[]string{"switzerland", "ch", "瑞士"}, "🇨🇭"},
		{[]string{"sweden", "se", "瑞典"}, "🇸🇪"},
		{[]string{"norway", "no", "挪威"}, "🇳🇴"},
		{[]string{"finland", "fi", "芬兰", "芬蘭"}, "🇫🇮"},
		{[]string{"denmark", "dk", "丹麦", "丹麥"}, "🇩🇰"},
		{[]string{"italy", "it", "意大利", "義大利"}, "🇮🇹"},
		{[]string{"spain", "es", "西班牙"}, "🇪🇸"},
		{[]string{"portugal", "pt", "葡萄牙"}, "🇵🇹"},
		{[]string{"poland", "pl", "波兰", "波蘭"}, "🇵🇱"},
		{[]string{"new zealand", "nz", "新西兰", "紐西蘭"}, "🇳🇿"},
		{[]string{"south africa", "za", "南非"}, "🇿🇦"},
		{[]string{"uae", "ae", "dubai", "阿联酋", "阿聯酋", "迪拜"}, "🇦🇪"},
		{[]string{"israel", "il", "以色列"}, "🇮🇱"},
	}
	for _, rule := range rules {
		for _, key := range rule.keys {
			if regionKeyMatches(text, key) {
				return rule.emoji
			}
		}
	}
	return ""
}

func regionKeyMatches(text, key string) bool {
	if len(key) == 2 || len(key) == 3 {
		matched, _ := regexp.MatchString(`(^|[^a-z0-9])`+regexp.QuoteMeta(key)+`([^a-z0-9]|$)`, text)
		return matched
	}
	return strings.Contains(text, key)
}
