package smtp

// Settings is the outbound mail server configuration.
type Settings struct {
	ID         string `gorm:"primaryKey;size:16" json:"-"`
	Enabled    bool   `gorm:"not null;default:false" json:"enabled"`
	Host       string `gorm:"size:200;not null;default:''" json:"host"`
	Port       int    `gorm:"not null;default:587" json:"port"`
	Encryption string `gorm:"size:16;not null;default:starttls" json:"encryption"`
	Username   string `gorm:"size:200;not null;default:''" json:"username"`
	Password   string `gorm:"not null;default:''" json:"-"`
	FromName   string `gorm:"size:120;not null;default:''" json:"fromName"`
	FromEmail  string `gorm:"size:200;not null;default:''" json:"fromEmail"`
}

const settingsID = "default"

// SettingsResponse is what the browser reads. Password is never returned.
type SettingsResponse struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Encryption  string `json:"encryption"`
	Username    string `json:"username"`
	HasPassword bool   `json:"hasPassword"`
	FromName    string `json:"fromName"`
	FromEmail   string `json:"fromEmail"`
}

type UpdateRequest struct {
	Enabled    bool   `json:"enabled"`
	Host       string `json:"host" binding:"max=200"`
	Port       int    `json:"port"`
	Encryption string `json:"encryption" binding:"max=16"`
	Username   string `json:"username" binding:"max=200"`
	Password   string `json:"password" binding:"max=500"`
	FromName   string `json:"fromName" binding:"max=120"`
	FromEmail  string `json:"fromEmail" binding:"max=200"`
}

type TestRequest struct {
	To string `json:"to" binding:"required,email"`
}

func (s Settings) toResponse() SettingsResponse {
	return SettingsResponse{
		Enabled:     s.Enabled,
		Host:        s.Host,
		Port:        s.Port,
		Encryption:  s.Encryption,
		Username:    s.Username,
		HasPassword: s.Password != "",
		FromName:    s.FromName,
		FromEmail:   s.FromEmail,
	}
}
