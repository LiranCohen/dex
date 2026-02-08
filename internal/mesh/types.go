package mesh

// Status represents the current mesh connection status.
type Status struct {
	// Connected indicates whether the mesh client is connected.
	Connected bool `json:"connected"`

	// MeshIP is the IP address assigned to this node on the mesh.
	MeshIP string `json:"mesh_ip,omitempty"`

	// Hostname is this node's hostname on the mesh.
	Hostname string `json:"hostname,omitempty"`

	// DERPRegion is the DERP relay region currently in use.
	// 0 means no DERP relay is being used (direct connection).
	DERPRegion int `json:"derp_region,omitempty"`

	// DERPLatency is the latency to the home DERP region in milliseconds.
	DERPLatency float64 `json:"derp_latency_ms,omitempty"`

	// IsHQ indicates whether this node is the HQ.
	IsHQ bool `json:"is_hq"`

	// Online indicates the node's online status.
	Online bool `json:"online"`

	// LastSeen is the last time this node was seen (ISO 8601).
	LastSeen string `json:"last_seen,omitempty"`

	// TunnelConnected indicates whether the tunnel to Ingress is connected.
	TunnelConnected bool `json:"tunnel_connected"`

	// TunnelEndpoints is the number of endpoints exposed via tunnel.
	TunnelEndpoints int `json:"tunnel_endpoints,omitempty"`
}

// Peer represents another Outpost node on the mesh network.
type Peer struct {
	// Hostname is the peer's hostname.
	Hostname string `json:"hostname"`

	// MeshIP is the peer's mesh IP address.
	MeshIP string `json:"mesh_ip,omitempty"`

	// Online indicates whether the peer is currently online.
	Online bool `json:"online"`

	// Direct indicates whether we have a direct connection to this peer.
	// If false, traffic is relayed through DERP.
	Direct bool `json:"direct"`

	// LastSeen is the last time this peer was seen (ISO 8601).
	LastSeen string `json:"last_seen,omitempty"`

	// IsHQ indicates whether this peer is the HQ.
	IsHQ bool `json:"is_hq,omitempty"`

	// Tags are the ACL tags assigned to this peer.
	Tags []string `json:"tags,omitempty"`
}
