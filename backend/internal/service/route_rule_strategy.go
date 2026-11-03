package service

import (
	"fmt"

	"github.com/ackwrap/ackrun/internal/model"
)

func (svc *RouteRuleService) SetProxyCollectionService(collections *ProxyCollectionService) {
	svc.proxyCollectionSvc = collections
}

func (svc *RouteRuleService) prepareNewRuleStrategy(req *model.RouteRuleRequest) (*model.ProxyCollection, error) {
	if req.Strategy == nil {
		return nil, nil
	}
	if svc.proxyCollectionSvc == nil {
		return nil, fmt.Errorf("代理策略服务不可用")
	}
	payload := *req.Strategy
	rule := &model.RouteRule{Name: req.Name, RuleType: req.RuleType, Outbound: req.Outbound}
	if err := svc.proxyCollectionSvc.prepareCollectionRuleBinding(&payload, rule, 0); err != nil {
		return nil, err
	}
	return prepareProxyCollection(&payload)
}
