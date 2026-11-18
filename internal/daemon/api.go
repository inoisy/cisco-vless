package daemon

// Типы запросов и ответов API демона — общие для CLI и GUI.

type Status struct {
	Cisco bool `json:"cisco"`
	// Cisco отвалилась сама (не cvpn off) — значок просит новый OTP.
	CiscoDropped bool     `json:"cisco_dropped,omitempty"`
	Exit         string   `json:"exit"`       // vless | wg | awg | ""
	Exits        []string `json:"exits"`      // какие выходы настроены
	Configured   bool     `json:"configured"` // cisco настроен
	IP           string   `json:"ip,omitempty"`
	CorpHost     string   `json:"corp_host,omitempty"`
	CorpIP       string   `json:"corp_ip,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
	OS           string   `json:"os"`
}

type CiscoOn struct {
	OTP      string `json:"otp,omitempty"`
	Password string `json:"password,omitempty"` // пусто — из файла (Linux)
}

// Settings — то, что правится в окне настроек и в cvpn setup.
type Settings struct {
	CiscoHost     string   `json:"cisco_host"`
	CiscoUser     string   `json:"cisco_user"`
	CiscoGroup    string   `json:"cisco_group"`
	CiscoPin      string   `json:"cisco_pin"`
	CorpDomains   []string `json:"corp_domains"`
	CorpCheckHost string   `json:"corp_check_host"`
	VLESSLink     string   `json:"vless_link"`
	// Ссылки из подписки, скачанные клиентом (cvpn setup): у клиента может
	// быть путь до сервера подписки, которого нет у службы без туннеля.
	VLESSLinks string `json:"vless_links,omitempty"`
	// То же для нескольких подписок: адрес → ссылки.
	VLESSSubs map[string]string `json:"vless_subs,omitempty"`
	BootExit  string            `json:"boot_exit"`
	// Содержимое конфигов; nil — не менять, "" — удалить.
	WGConf  *string `json:"wg_conf,omitempty"`
	AWGConf *string `json:"awg_conf,omitempty"`
	// Пути к программам, пусто — искать самим.
	Openconnect string `json:"openconnect"`
	VPNCScript  string `json:"vpnc_script"`
	SingBox     string `json:"sing_box"`
	// Только в ответе.
	HasWG       bool `json:"has_wg"`
	HasAWG      bool `json:"has_awg"`
	HasPassword bool `json:"has_password"` // Linux: файл с паролем есть
}

type Probe struct {
	Host string `json:"host"`
}

type ProbeResult struct {
	IP  string `json:"ip"`
	Pin string `json:"pin"`
}

type Password struct {
	Password string `json:"password"`
}

type Message struct {
	Message string `json:"message"`
}

type Logs struct {
	Cisco  string `json:"cisco"`
	VLESS  string `json:"vless"`
	Daemon string `json:"daemon"`
}
