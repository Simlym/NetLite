package certificates

import (
	"encoding/json"
	"net/http"
	"netlite/internal/model"
	"os"

	"github.com/go-chi/chi/v5"
)

func certJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (m *Manager) current(id string) (model.Certificate, bool) {
	cfg, err := m.db.Load()
	if err != nil {
		return model.Certificate{}, false
	}
	for _, c := range cfg.Certificates {
		if c.ID == id {
			return c, true
		}
	}
	return model.Certificate{}, false
}
func (m *Manager) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := m.db.Load()
		if err != nil {
			certJSON(w, 500, map[string]string{"error": "无法读取证书配置"})
			return
		}
		states := map[string]State{}
		for _, c := range cfg.Certificates {
			s, err := m.Status(c)
			if err != nil {
				certJSON(w, 500, map[string]string{"error": "无法读取证书状态"})
				return
			}
			states[c.ID] = s
		}
		certJSON(w, 200, map[string]any{"certificates": states, "http_challenge_address": m.HTTP.Address})
	})
	r.Post("/{id}/issue", func(w http.ResponseWriter, r *http.Request) {
		c, ok := m.current(chi.URLParam(r, "id"))
		if !ok {
			certJSON(w, 404, map[string]string{"error": "证书不存在"})
			return
		}
		if err := m.Request(c, true); err != nil {
			certJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		certJSON(w, 202, map[string]string{"message": "签发或续期已排队"})
	})
	r.Get("/root", func(w http.ResponseWriter, r *http.Request) {
		b, err := m.RootCertificate()
		if err != nil {
			certJSON(w, 500, map[string]string{"error": "内部 CA 不可用"})
			return
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", "attachment; filename=netlite-root-ca.pem")
		_, _ = w.Write(b)
	})
	r.Get("/{id}/certificate", func(w http.ResponseWriter, r *http.Request) {
		c, ok := m.current(chi.URLParam(r, "id"))
		if !ok {
			certJSON(w, 404, map[string]string{"error": "证书不存在"})
			return
		}
		files, err := m.Files(c)
		if err != nil {
			certJSON(w, 409, map[string]string{"error": "证书尚未就绪"})
			return
		}
		b, err := os.ReadFile(files.CertificateFile)
		if err != nil {
			certJSON(w, 500, map[string]string{"error": "无法读取证书"})
			return
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", "attachment; filename=certificate.pem")
		_, _ = w.Write(b)
	})
	return r
}
