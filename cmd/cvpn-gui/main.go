// cvpn-gui — значок в трее и окно (статус, настройки, логи) поверх службы
// cvpn. Сам ничего не поднимает: всё делает служба, GUI — её клиент, как
// и CLI.
package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/inoisy/cisco-vless/internal/client"
	"github.com/inoisy/cisco-vless/internal/config"
	"github.com/inoisy/cisco-vless/internal/ipc"
)

//go:embed frontend
var frontend embed.FS

func main() {
	g := &gui{c: client.New()}

	// Разработка: то же окно в обычном браузере, без webview и трея.
	if len(os.Args) == 3 && os.Args[1] == "-serve" {
		log.Printf("http://%s", os.Args[2])
		g.tray, g.app = nil, nil
		log.Fatal(http.ListenAndServe(os.Args[2], g.handler()))
	}

	app := application.New(application.Options{
		Name:        "cvpn",
		Description: "Cisco + VLESS / WireGuard / AmneziaWG",
		Assets:      application.AssetOptions{Handler: g.handler(), DisableLogging: true},
		Mac: application.MacOptions{
			// Только значок в menu bar, без Dock.
			ActivationPolicy: application.ActivationPolicyAccessory,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.github.inoisy.cvpn",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				g.show("status")
			},
		},
	})
	g.app = app

	g.win = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "cvpn",
		Width:  560,
		Height: 720,
		Hidden: true,
		URL:    "/",
	})
	// Крестик прячет окно, а не выходит: живём в трее.
	g.win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		g.win.Hide()
	})

	g.tray = app.SystemTray.New()
	g.tray.SetTooltip("cvpn")
	g.tray.SetIcon(trayIcon(stateUnknown))
	g.buildMenu(nil)
	go g.poll()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

type gui struct {
	c    *client.Client
	app  *application.App
	win  *application.WebviewWindow
	tray *application.SystemTray
}

// show открывает окно на вкладке (status | settings | logs).
func (g *gui) show(tab string) {
	application.InvokeAsync(func() {
		g.win.Show()
		g.win.Focus()
		if tab != "" {
			g.win.ExecJS("window.cvpnShow && window.cvpnShow(" + jsString(tab) + ")")
		}
	})
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// handler: статика из frontend/ и /api/* → служба. Подключение cisco и
// пароль идут через client: на macOS пароль берётся из Keychain
// пользователя и в webview не попадает.
func (g *gui) handler() http.Handler {
	mux := http.NewServeMux()

	static, _ := fs.Sub(frontend, "frontend")
	mux.Handle("/", http.FileServerFS(static))

	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(&url.URL{Scheme: "http", Host: "cvpn"})
			r.Out.URL.Path = strings.TrimPrefix(r.In.URL.Path, "/api")
			r.Out.Host = "cvpn"
		},
		Transport: ipc.Transport(config.Default().Socket),
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeErr(w, "служба cvpn недоступна — установлена и запущена? ("+err.Error()+")")
		},
		ModifyResponse: func(resp *http.Response) error {
			if resp.Request.Method != http.MethodGet {
				go g.refresh() // что-то включили/выключили — обновить трей
			}
			return nil
		},
	}
	mux.Handle("/api/", proxy)

	mux.HandleFunc("POST /api/on", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Exit, OTP string }
		json.NewDecoder(r.Body).Decode(&req)
		g.replyMsg(w)(g.c.On(req.Exit, req.OTP))
	})
	mux.HandleFunc("POST /api/cisco/on", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ OTP string }
		json.NewDecoder(r.Body).Decode(&req)
		g.replyMsg(w)(g.c.CiscoOn(req.OTP))
	})
	mux.HandleFunc("POST /api/password", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Password string }
		json.NewDecoder(r.Body).Decode(&req)
		s, err := g.c.Settings()
		if err == nil {
			err = g.c.SetPassword(s.CiscoUser, req.Password)
		}
		g.replyMsg(w)("пароль сохранён", err)
	})
	mux.HandleFunc("GET /api/password", func(w http.ResponseWriter, r *http.Request) {
		s, err := g.c.Settings()
		if err != nil {
			writeErr(w, err.Error())
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"has": g.c.HasPassword(s)})
	})
	return mux
}

func (g *gui) replyMsg(w http.ResponseWriter) func(string, error) {
	return func(msg string, err error) {
		go g.refresh()
		if err != nil {
			if msg != "" {
				msg += "\n"
			}
			writeErr(w, msg+err.Error())
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"message": msg})
	}
}

func writeErr(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
