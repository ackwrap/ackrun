package service

import (
	"strings"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

type NodeGroupService struct {
	store *store.Store
}

func NewNodeGroupService(s *store.Store) *NodeGroupService {
	return &NodeGroupService{store: s}
}

func (svc *NodeGroupService) List() ([]model.NodeGroupWithStats, error) {
	return svc.store.ListNodeGroups()
}

func (svc *NodeGroupService) Get(id int64) (*model.NodeGroup, error) {
	return svc.store.GetNodeGroup(id)
}

func (svc *NodeGroupService) Create(req *model.NodeGroupRequest) (*model.NodeGroup, error) {
	return svc.store.CreateNodeGroup(req)
}

func (svc *NodeGroupService) Update(id int64, req *model.NodeGroupRequest) error {
	return svc.store.UpdateNodeGroup(id, req)
}

func (svc *NodeGroupService) Delete(id int64) error {
	return svc.store.DeleteNodeGroup(id)
}

func (svc *NodeGroupService) BatchDelete(ids []int64) error {
	return svc.store.DeleteNodeGroups(ids)
}

func (svc *NodeGroupService) Reorder(ids []int64) error {
	return svc.store.ReorderNodeGroups(ids)
}

func (svc *NodeGroupService) PreviewMatches(filterProtocols, filterSubscriptions, filterInclude, filterExclude string) ([]model.Node, error) {
	return svc.store.PreviewNodeGroupMatches(filterProtocols, filterSubscriptions, filterInclude, filterExclude)
}

// QuickSetup 一键快速配置（只创建有节点的地域）
func (svc *NodeGroupService) QuickSetup(req model.NodeGroupQuickSetupRequest) error {
	allNodes, err := svc.store.PreviewNodeGroupMatches(req.FilterProtocols, req.FilterSubscriptions, ".*", "")
	if err != nil {
		return err
	}
	logging.Info("node_group.quick_setup", "智能快速配置开始，参与匹配的启用节点数: %d，订阅筛选: %s，协议筛选: %s", len(allNodes), req.FilterSubscriptions, req.FilterProtocols)

	// 预设节点组模板（覆盖世界各地）
	templates := []model.NodeGroupRequest{
		// 亚洲地区
		{Name: "香港节点", Type: "selector", FilterInclude: "🇭🇰|HK|hk|香港|港|HongKong|Hong Kong", FilterExclude: "免费|过期|流量|官网|到期", Enabled: true, Priority: 0},
		{Name: "台湾节点", Type: "selector", FilterInclude: "🇹🇼|TW|tw|台湾|台|Taiwan", FilterExclude: "免费|过期", Enabled: true, Priority: 1},
		{Name: "日本节点", Type: "selector", FilterInclude: "🇯🇵|JP|jp|日本|日|Japan", FilterExclude: "免费|过期", Enabled: true, Priority: 2},
		{Name: "韩国节点", Type: "selector", FilterInclude: "🇰🇷|KR|kr|韩国|韩|Korea", FilterExclude: "免费|过期", Enabled: true, Priority: 3},
		{Name: "新加坡节点", Type: "selector", FilterInclude: "🇸🇬|SG|sg|新加坡|坡|狮城|Singapore", FilterExclude: "免费|过期", Enabled: true, Priority: 4},
		{Name: "印度节点", Type: "selector", FilterInclude: "🇮🇳|IN|in|印度|India", FilterExclude: "免费", Enabled: true, Priority: 5},
		{Name: "泰国节点", Type: "selector", FilterInclude: "🇹🇭|TH|th|泰国|Thailand", FilterExclude: "免费", Enabled: true, Priority: 6},
		{Name: "越南节点", Type: "selector", FilterInclude: "🇻🇳|VN|vn|越南|Vietnam", FilterExclude: "免费", Enabled: true, Priority: 7},
		{Name: "菲律宾节点", Type: "selector", FilterInclude: "🇵🇭|PH|ph|菲律宾|Philippines", FilterExclude: "免费", Enabled: true, Priority: 8},

		// 美洲地区
		{Name: "美国节点", Type: "selector", FilterInclude: "🇺🇸|US|us|美国|美|United States|America", FilterExclude: "免费|过期", Enabled: true, Priority: 10},
		{Name: "加拿大节点", Type: "selector", FilterInclude: "🇨🇦|CA|ca|加拿大|Canada", FilterExclude: "免费", Enabled: true, Priority: 11},
		{Name: "巴西节点", Type: "selector", FilterInclude: "🇧🇷|BR|br|巴西|Brazil", FilterExclude: "免费", Enabled: true, Priority: 12},
		{Name: "阿根廷节点", Type: "selector", FilterInclude: "🇦🇷|AR|ar|阿根廷|Argentina", FilterExclude: "免费", Enabled: true, Priority: 13},
		{Name: "墨西哥节点", Type: "selector", FilterInclude: "🇲🇽|MX|mx|墨西哥|Mexico", FilterExclude: "免费", Enabled: true, Priority: 14},

		// 欧洲地区
		{Name: "英国节点", Type: "selector", FilterInclude: "🇬🇧|UK|uk|英国|英|United Kingdom|Britain", FilterExclude: "免费", Enabled: true, Priority: 20},
		{Name: "法国节点", Type: "selector", FilterInclude: "🇫🇷|FR|fr|法国|France", FilterExclude: "免费", Enabled: true, Priority: 21},
		{Name: "德国节点", Type: "selector", FilterInclude: "🇩🇪|DE|de|德国|德|Germany", FilterExclude: "免费", Enabled: true, Priority: 22},
		{Name: "荷兰节点", Type: "selector", FilterInclude: "🇳🇱|NL|nl|荷兰|Netherlands", FilterExclude: "免费", Enabled: true, Priority: 23},
		{Name: "瑞士节点", Type: "selector", FilterInclude: "🇨🇭|CH|ch|瑞士|Switzerland", FilterExclude: "免费", Enabled: true, Priority: 24},
		{Name: "瑞典节点", Type: "selector", FilterInclude: "🇸🇪|SE|se|瑞典|Sweden", FilterExclude: "免费", Enabled: true, Priority: 25},
		{Name: "挪威节点", Type: "selector", FilterInclude: "🇳🇴|NO|no|挪威|Norway", FilterExclude: "免费", Enabled: true, Priority: 26},
		{Name: "芬兰节点", Type: "selector", FilterInclude: "🇫🇮|FI|fi|芬兰|Finland", FilterExclude: "免费", Enabled: true, Priority: 27},
		{Name: "丹麦节点", Type: "selector", FilterInclude: "🇩🇰|DK|dk|丹麦|Denmark", FilterExclude: "免费", Enabled: true, Priority: 28},
		{Name: "意大利节点", Type: "selector", FilterInclude: "🇮🇹|IT|it|意大利|Italy", FilterExclude: "免费", Enabled: true, Priority: 29},
		{Name: "西班牙节点", Type: "selector", FilterInclude: "🇪🇸|ES|es|西班牙|Spain", FilterExclude: "免费", Enabled: true, Priority: 30},
		{Name: "葡萄牙节点", Type: "selector", FilterInclude: "🇵🇹|PT|pt|葡萄牙|Portugal", FilterExclude: "免费", Enabled: true, Priority: 31},
		{Name: "波兰节点", Type: "selector", FilterInclude: "🇵🇱|PL|pl|波兰|Poland", FilterExclude: "免费", Enabled: true, Priority: 32},
		{Name: "俄罗斯节点", Type: "selector", FilterInclude: "🇷🇺|RU|ru|俄罗斯|俄|Russia", FilterExclude: "免费", Enabled: true, Priority: 33},
		{Name: "土耳其节点", Type: "selector", FilterInclude: "🇹🇷|TR|tr|土耳其|Turkey", FilterExclude: "免费", Enabled: true, Priority: 34},

		// 大洋洲地区
		{Name: "澳大利亚节点", Type: "selector", FilterInclude: "🇦🇺|AU|au|澳大利亚|澳洲|Australia", FilterExclude: "免费|AUS", Enabled: true, Priority: 40},
		{Name: "新西兰节点", Type: "selector", FilterInclude: "🇳🇿|NZ|nz|新西兰|New Zealand", FilterExclude: "免费", Enabled: true, Priority: 41},

		// 非洲/中东地区
		{Name: "南非节点", Type: "selector", FilterInclude: "🇿🇦|ZA|za|南非|South Africa", FilterExclude: "免费", Enabled: true, Priority: 50},
		{Name: "阿联酋节点", Type: "selector", FilterInclude: "🇦🇪|AE|ae|阿联酋|迪拜|Dubai|UAE", FilterExclude: "免费", Enabled: true, Priority: 51},
		{Name: "以色列节点", Type: "selector", FilterInclude: "🇮🇱|IL|il|以色列|Israel", FilterExclude: "免费", Enabled: true, Priority: 52},

		// 特殊节点组（始终创建）
		{Name: "自动选择", Type: "urltest", FilterInclude: ".*", FilterExclude: "免费|过期|流量|官网|到期|剩余|套餐|订阅", Enabled: true, Priority: 100, TestInterval: 600, Tolerance: 100},
		{Name: "全部节点", Type: "selector", FilterInclude: ".*", FilterExclude: "", Enabled: true, Priority: 101},
	}
	existingGroups, err := svc.store.ListNodeGroups()
	if err != nil {
		return err
	}
	existingNames := make(map[string]bool, len(existingGroups))
	for _, group := range existingGroups {
		existingNames[group.Name] = true
	}

	createdCount := 0
	skippedExistingCount := 0
	for _, tmpl := range templates {
		tmpl.FilterProtocols = req.FilterProtocols
		tmpl.FilterSubscriptions = req.FilterSubscriptions
		if existingNames[tmpl.Name] {
			skippedExistingCount++
			continue
		}

		// 预览匹配节点数
		matchedNodes, err := svc.store.PreviewNodeGroupMatches(tmpl.FilterProtocols, tmpl.FilterSubscriptions, tmpl.FilterInclude, tmpl.FilterExclude)
		if err != nil {
			continue
		}

		// 只创建有节点的地域组（特殊节点组除外，始终创建）
		if len(matchedNodes) == 0 && tmpl.Priority < 100 {
			continue
		}

		if _, err := svc.store.CreateNodeGroup(&tmpl); err != nil {
			if isNodeGroupDuplicateName(err) {
				skippedExistingCount++
				continue
			}
			return err
		}
		createdCount++
	}

	logging.Info("node_group.quick_setup", "智能快速配置完成，创建节点组数: %d，已存在跳过: %d，参与匹配的启用节点数: %d，订阅筛选: %s，协议筛选: %s", createdCount, skippedExistingCount, len(allNodes), req.FilterSubscriptions, req.FilterProtocols)
	return nil
}

func isNodeGroupDuplicateName(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") && strings.Contains(msg, "node_groups") && strings.Contains(msg, "name")
}
