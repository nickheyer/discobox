// Package balancer implements load balancing algorithms for Discobox
package balancer

import (
	"net/url"
	"discobox/internal/types"
)

// NewServer creates a new server instance from a service endpoint
func NewServer(endpoint string, serviceID string, weight int) (*types.Server, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	
	return &types.Server{
		URL:      u,
		ID:       serviceID + "-" + u.Host,
		Weight:   weight,
		Healthy:  true,
		Metadata: make(map[string]string),
	}, nil
}

// ServersFromService creates servers from a service definition
func ServersFromService(service *types.Service) ([]*types.Server, error) {
	servers := make([]*types.Server, 0, len(service.Endpoints))
	
	for _, endpoint := range service.Endpoints {
		server, err := NewServer(endpoint, service.ID, service.Weight)
		if err != nil {
			return nil, err
		}
		server.MaxConns = service.MaxConns
		servers = append(servers, server)
	}
	
	return servers, nil
}
