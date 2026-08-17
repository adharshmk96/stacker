package serversettings

import "time"

type Instance struct {
	Hostname  string    `json:"hostname"`
	Version   string    `json:"version"`
	BuiltAt   string    `json:"builtAt,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	Docker    string    `json:"docker,omitempty"`
	OS        string    `json:"os,omitempty"`
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
