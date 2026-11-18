package singbox

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// URLTest — проверка живости серверов группы: полный запрос через сервер, как
// пинг в Happ. tolerance — пока текущий сервер не медленнее лучшего на столько,
// не переключаемся: соединения не рвутся из-за дрожания задержки.
const (
	urlTestURL       = "https://www.gstatic.com/generate_204"
	urlTestInterval  = "1m"
	urlTestTolerance = 150
)

// Outbounds — outbound'ы из ссылок. Одна ссылка — outbound proxy, ошибка
// ссылки — ошибка. Несколько — каждая под своим именем (из #имени), плюс
// группа urltest с тегом proxy; ссылки, которые не разобрались, пропускаются,
// ошибка — только если не осталось ни одной.
func Outbounds(links string) ([]any, error) {
	list := strings.Fields(links)
	if len(list) == 0 {
		return nil, fmt.Errorf("нет ссылки vless://")
	}
	if len(list) == 1 {
		out, err := outbound(list[0], "proxy")
		if err != nil {
			return nil, err
		}
		return []any{out}, nil
	}

	var outs []any
	var tags []string
	var skipped []string
	seen := map[string]bool{"proxy": true, "direct": true}
	for i, l := range list {
		tag := linkName(l)
		if tag == "" {
			tag = "server-" + strconv.Itoa(i+1)
		}
		for base, n := tag, 2; seen[tag]; n++ {
			tag = base + "-" + strconv.Itoa(n)
		}
		out, err := outbound(l, tag)
		if err != nil {
			skipped = append(skipped, tag+": "+err.Error())
			continue
		}
		seen[tag] = true
		outs = append(outs, out)
		tags = append(tags, tag)
	}
	if len(outs) == 0 {
		return nil, fmt.Errorf("ни одна ссылка не подошла: %s", strings.Join(skipped, "; "))
	}
	group := map[string]any{
		"type":      "urltest",
		"tag":       "proxy",
		"outbounds": tags,
		"url":       urlTestURL,
		"interval":  urlTestInterval,
		"tolerance": urlTestTolerance,
	}
	return append([]any{group}, outs...), nil
}

func outbound(link, tag string) (map[string]any, error) {
	switch {
	case strings.HasPrefix(link, "vless://"):
		return vlessOutbound(link, tag)
	case strings.HasPrefix(link, "hysteria2://"), strings.HasPrefix(link, "hy2://"):
		return hy2Outbound(link, tag)
	}
	return nil, fmt.Errorf("не похоже на ссылку vless://uuid@host:port?...")
}

func linkName(link string) string {
	_, frag, ok := strings.Cut(link, "#")
	if !ok {
		return ""
	}
	return strings.TrimSpace(unesc(frag))
}

// hy2Outbound — outbound из ссылки hysteria2://пароль@host:port?sni=&insecure=1&obfs=salamander&obfs-password=.
func hy2Outbound(link, tag string) (map[string]any, error) {
	u, err := url.Parse(link)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("плохая ссылка hysteria2")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("плохой порт %q", u.Port())
	}
	pass := u.User.Username()
	if p, ok := u.User.Password(); ok {
		pass += ":" + p
	}
	if pass == "" {
		return nil, fmt.Errorf("в ссылке hysteria2 нет пароля")
	}
	q := u.Query()
	tls := map[string]any{"enabled": true}
	if sni := q.Get("sni"); sni != "" {
		tls["server_name"] = sni
	}
	if q.Get("insecure") == "1" {
		tls["insecure"] = true
	}
	out := map[string]any{
		"type":        "hysteria2",
		"tag":         tag,
		"server":      u.Hostname(),
		"server_port": port,
		"password":    pass,
		"tls":         tls,
	}
	if o := q.Get("obfs"); o != "" {
		out["obfs"] = map[string]any{"type": o, "password": q.Get("obfs-password")}
	}
	return out, nil
}

// IsSubscription — в настройке vless ровно один адрес подписки.
func IsSubscription(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "https://") && !strings.ContainsAny(s, " \n\t")
}

// Sources делит настройку vless на подписки (https://) и ссылки. Через пробел
// в любом сочетании: подписка своего сервера плюс ссылки или подписки с
// другими выходами — упал один выход, urltest уходит на другой.
func Sources(spec string) (subs, links []string) {
	for _, f := range strings.Fields(spec) {
		if strings.HasPrefix(f, "https://") {
			subs = append(subs, f)
		} else {
			links = append(links, f)
		}
	}
	return subs, links
}

// CheckSources — каждая запись настройки vless — ссылка vless://,
// hysteria2:// (hy2://) или адрес подписки https://.
func CheckSources(spec string) error {
	for _, f := range strings.Fields(spec) {
		switch {
		case strings.HasPrefix(f, "vless://"), strings.HasPrefix(f, "hysteria2://"),
			strings.HasPrefix(f, "hy2://"), strings.HasPrefix(f, "https://"):
		default:
			scheme, _, _ := strings.Cut(f, "://")
			if len(scheme) > 12 {
				scheme = scheme[:12] + "…"
			}
			return fmt.Errorf("%q: нужна ссылка vless://, hysteria2:// или адрес подписки https://", scheme)
		}
	}
	return nil
}

// HasSubscription — в настройке есть хотя бы одна подписка.
func HasSubscription(spec string) bool {
	subs, _ := Sources(spec)
	return len(subs) > 0
}

// Resolve — ссылки для sing-box из настройки vless. Каждая подписка качается,
// копия — в cache(адрес); не скачалась — берётся копия, нет и копии —
// подписка пропускается. warns — что не скачалось; ошибка — только когда не
// осталось ни одной ссылки.
func Resolve(spec string, cache func(subURL string) string) (links string, warns []string, err error) {
	subs, list := Sources(spec)
	return resolveList(subs, list, cache)
}

func resolveList(subs, list []string, cache func(string) string) (links string, warns []string, err error) {
	for _, u := range subs {
		got, serr := Subscription(u, cache(u))
		if serr != nil {
			warns = append(warns, serr.Error())
		}
		list = append(list, strings.Fields(got)...)
	}
	if len(list) == 0 {
		return "", warns, fmt.Errorf("нет ни одной ссылки: %s", strings.Join(warns, "; "))
	}
	return strings.Join(list, "\n") + "\n", warns, nil
}

// FetchSubscriptions качает все подписки настройки vless — от пользователя
// (cvpn setup, значок): у него бывает путь до сервера подписки, которого нет
// у службы без туннеля. got — адрес → ссылки; warns — что не скачалось.
func FetchSubscriptions(spec string) (got map[string]string, warns []string) {
	subs, _ := Sources(spec)
	got = map[string]string{}
	for _, u := range subs {
		links, err := FetchSubscription(u)
		if err != nil {
			warns = append(warns, fmt.Sprintf("подписка %s не скачалась (%v) — служба попробует сама при vless on", SubHost(u), err))
			continue
		}
		got[u] = links
	}
	return got, warns
}

// SubHost — адрес подписки без пути: в пути токен, в логи и вывод — только хост.
func SubHost(subURL string) string {
	if u, err := url.Parse(subURL); err == nil && u.Host != "" {
		return u.Host
	}
	return "подписка"
}

// Subscription качает подписку (base64 или текст, ссылка на строку) и кладёт
// копию в cache. Не скачалась — отдаёт копию: подписка лежит на том же
// сервере, до которого может не быть пути, а качаем до старта туннеля.
func Subscription(subURL, cache string) (string, error) {
	links, err := fetchSubscription(subURL, 20*time.Second)
	if err == nil {
		_ = os.WriteFile(cache, []byte(links), 0o600) // копия — на случай, когда не скачается
		return links, nil
	}
	b, cerr := os.ReadFile(cache)
	if cerr != nil || len(b) == 0 {
		return "", fmt.Errorf("подписка %s: %w (и нет сохранённой копии)", SubHost(subURL), err)
	}
	return string(b), fmt.Errorf("подписка %s: %w — беру сохранённую копию", SubHost(subURL), err)
}

// FetchSubscription — ссылки из подписки без копии на диске.
func FetchSubscription(subURL string) (string, error) {
	return fetchSubscription(subURL, 20*time.Second)
}

func fetchSubscription(subURL string, timeout time.Duration) (string, error) {
	c := &http.Client{Timeout: timeout}
	resp, err := c.Get(subURL)
	if err != nil {
		if ue, ok := err.(*url.Error); ok { // в url.Error — адрес с токеном
			return "", ue.Err
		}
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(b))
	if !strings.Contains(text, "://") {
		dec, err := decodeB64(text)
		if err != nil {
			return "", fmt.Errorf("не base64 и не список ссылок")
		}
		text = dec
	}
	var links []string
	for _, l := range strings.Fields(text) {
		if strings.Contains(l, "://") {
			links = append(links, l)
		}
	}
	if len(links) == 0 {
		return "", fmt.Errorf("в подписке нет ссылок")
	}
	return strings.Join(links, "\n") + "\n", nil
}

func decodeB64(s string) (string, error) {
	s = strings.Join(strings.Fields(s), "")
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("не base64")
}
