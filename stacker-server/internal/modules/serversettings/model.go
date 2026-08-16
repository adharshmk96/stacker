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
	Instance Instance `json:"instance"`
	Domain   string   `json:"domain"`
}

type DomainRequest struct {
	Domain string `json:"domain" binding:"required"`
}

type RestartRequest struct {
	Target string `json:"target" binding:"required"`
}
