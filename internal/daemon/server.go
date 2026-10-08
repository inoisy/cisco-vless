package daemon

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
)

// Serve — HTTP API демона на listener (сокет/pipe).
//
//	GET  /status[?ip=1]         Status
//	POST /on   {exit, otp, password}
//	POST /off
//	POST /cisco/on {otp, password}   POST /cisco/off
//	POST /exit/{vless|wg|awg}/on     POST /exit/{kind}/off   POST /exit/off
//	GET  /settings   PUT /settings
//	POST /probe {host}   POST /password {password}   GET /logs
func (m *Manager) Serve(l net.Listener) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		reply(w, m.Status(r.URL.Query().Get("ip") != ""), nil)
	})
	mux.HandleFunc("POST /on", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Exit string `json:"exit"`
			CiscoOn
		}
		if !decode(w, r, &req) {
			return
		}
		replyMsg(w, func() (string, error) { return m.On(req.Exit, req.CiscoOn) })
	})
	mux.HandleFunc("POST /off", func(w http.ResponseWriter, r *http.Request) {
		replyMsg(w, func() (string, error) { return m.Off(), nil })
	})
	mux.HandleFunc("POST /cisco/on", func(w http.ResponseWriter, r *http.Request) {
		var req CiscoOn
		if !decode(w, r, &req) {
			return
		}
		replyMsg(w, func() (string, error) { return m.CiscoOn(req) })
	})
	mux.HandleFunc("POST /cisco/off", func(w http.ResponseWriter, r *http.Request) {
		replyMsg(w, func() (string, error) { return m.CiscoOff(), nil })
	})
	mux.HandleFunc("POST /exit/{kind}/on", func(w http.ResponseWriter, r *http.Request) {
		replyMsg(w, func() (string, error) { return m.ExitOn(r.PathValue("kind")) })
	})
	mux.HandleFunc("POST /exit/{kind}/off", func(w http.ResponseWriter, r *http.Request) {
		replyMsg(w, func() (string, error) { return m.ExitOff(r.PathValue("kind")), nil })
	})
	mux.HandleFunc("POST /exit/off", func(w http.ResponseWriter, r *http.Request) {
		replyMsg(w, func() (string, error) { return m.ExitOff(""), nil })
	})
	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		reply(w, m.Settings(), nil)
	})
	mux.HandleFunc("PUT /settings", func(w http.ResponseWriter, r *http.Request) {
		var s Settings
		if !decode(w, r, &s) {
			return
		}
		if err := m.SetSettings(s); err != nil {
			reply(w, nil, err)
			return
		}
		reply(w, m.Settings(), nil)
	})
	mux.HandleFunc("POST /probe", func(w http.ResponseWriter, r *http.Request) {
		var p Probe
		if !decode(w, r, &p) {
			return
		}
		res, err := m.Probe(p.Host)
		reply(w, res, err)
	})
	mux.HandleFunc("POST /password", func(w http.ResponseWriter, r *http.Request) {
		var p Password
		if !decode(w, r, &p) {
			return
		}
		replyMsg(w, func() (string, error) { return "пароль сохранён", m.SetPassword(p.Password) })
	})
	mux.HandleFunc("GET /logs", func(w http.ResponseWriter, r *http.Request) {
		reply(w, m.Logs(), nil)
	})

	return (&http.Server{Handler: mux}).Serve(l)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err == nil && len(b) > 0 {
		err = json.Unmarshal(b, v)
	}
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "плохой запрос: " + err.Error()})
		return false
	}
	return true
}

func replyMsg(w http.ResponseWriter, f func() (string, error)) {
	msg, err := f()
	if err != nil && msg != "" {
		err = &msgErr{msg + "\n" + err.Error()}
	}
	if msg != "" {
		log.Print(msg)
	}
	reply(w, Message{msg}, err)
}

type msgErr struct{ s string }

func (e *msgErr) Error() string { return e.s }

func reply(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(v)
}
