package serversettings

import "time"

type Instance struct {
	Hostname   string    `json:"hostname"`
	IP         string    `json:"ip,omitempty"`
	Version    string    `json:"version"`
	BuiltAt    string    `json:"builtAt,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
	Docker     string    `json:"docker,omitempty"`
	OS         string    `json:"os,omitempty"`
	Revision   string    `json:"revision,omitempty"`
	Repository string    `json:"repository,omitempty"`
}

type Settings struct {
	Instance Instance    `json:"instance"`
	Traefik  TraefikInfo `json:"traefik"`
}

type TraefikInfo struct {
	Domain              string      `json:"domain"`
	HTTPS               bool        `json:"https"`
	CertificateResolver string      `json:"certificateResolver,omitempty"`
	BackendTarget       string      `json:"backendTarget,omitempty"`
	HTTPRedirect        bool        `json:"httpRedirect"`
	PublishedPorts      []string    `json:"publishedPorts"`
	StackName           string      `json:"stackName"`
	StackerService      ServiceInfo `json:"stackerService"`
	TraefikService      ServiceInfo `json:"traefikService"`
}

type ServiceInfo struct {
	Name      string    `json:"name"`
	Image     string    `json:"image,omitempty"`
	Version   string    `json:"version,omitempty"`
	Running   int       `json:"running"`
	Desired   int       `json:"desired"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

type DomainRequest struct {
	Domain string `json:"domain" binding:"required"`
}

type RestartRequest struct {
	Target string `json:"target" binding:"required"`
}

type UpdateCandidate struct {
	Channel     string    `json:"channel"`
	Version     string    `json:"version"`
	Revision    string    `json:"revision"`
	PublishedAt time.Time `json:"publishedAt,omitempty"`
	Available   bool      `json:"available"`
}

type Updates struct {
	Stable   UpdateCandidate `json:"stable"`
	Edge     UpdateCandidate `json:"edge"`
	Updating bool            `json:"updating"`
	Error    string          `json:"error,omitempty"`
}

type UpdateRequest struct {
	Channel string `json:"channel" binding:"required,oneof=stable edge"`
}
