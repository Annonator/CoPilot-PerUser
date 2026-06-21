package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"copilot-per-user/api/internal/auth"
	"copilot-per-user/api/internal/identity"
	"copilot-per-user/api/internal/usage"
)

type UsageService interface {
	GetMonthlyUsage(ctx context.Context, email string, year int, month int) (usage.MonthlyUsage, error)
}

type ServerConfig struct {
	Auth                       auth.Manager
	CompanyEmailDomains        []string
	Usage                      UsageService
	Now                        func() time.Time
	UsageReportingWindowMonths int
}

type Server struct {
	mux                        *http.ServeMux
	auth                       auth.Manager
	companyEmailDomains        []string
	usage                      UsageService
	now                        func() time.Time
	usageReportingWindowMonths int
}

func NewServer(cfg ServerConfig) *Server {
	server := &Server{
		mux:                        http.NewServeMux(),
		auth:                       cfg.Auth,
		companyEmailDomains:        append([]string(nil), cfg.CompanyEmailDomains...),
		usage:                      cfg.Usage,
		now:                        cfg.Now,
		usageReportingWindowMonths: cfg.UsageReportingWindowMonths,
	}
	if server.now == nil {
		server.now = time.Now
	}
	server.mux.HandleFunc("/healthz", server.handleHealthz)
	server.mux.HandleFunc("/v1/me", server.handleMe)
	server.mux.HandleFunc("/v1/usage", server.handleUsage)
	return server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	claims, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"email": claims.Email,
		"name":  claims.Name,
	})
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	claims, ok := s.authenticate(w, r)
	if !ok {
		return
	}

	year, month, err := parsePeriod(r, s.now(), s.usageReportingWindowMonths)
	if err != nil {
		writePeriodError(w, err)
		return
	}

	monthlyUsage, err := s.usage.GetMonthlyUsage(r.Context(), claims.Email, year, month)
	if err != nil {
		if errors.Is(err, identity.ErrIdentityNotFound) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		if errors.Is(err, usage.ErrInvalidPeriod) || errors.Is(err, usage.ErrPeriodOutOfRange) {
			writePeriodError(w, err)
			return
		}
		writeError(w, http.StatusBadGateway, "bad_gateway")
		return
	}
	writeJSON(w, http.StatusOK, monthlyUsage)
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (auth.Claims, bool) {
	claims, err := s.auth.Validate(r.Header.Get("Authorization"))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return auth.Claims{}, false
	}
	if !auth.AllowedDomain(claims.Email, s.companyEmailDomains) {
		writeError(w, http.StatusForbidden, "forbidden")
		return auth.Claims{}, false
	}
	return claims, true
}

func parsePeriod(r *http.Request, now time.Time, reportingWindowMonths int) (int, int, error) {
	query := r.URL.Query()
	year, err := strconv.Atoi(query.Get("year"))
	if err != nil || year < 2000 {
		return 0, 0, usage.ErrInvalidPeriod
	}
	month, err := strconv.Atoi(query.Get("month"))
	if err != nil || month < 1 || month > 12 {
		return 0, 0, usage.ErrInvalidPeriod
	}
	if err := usage.ValidateReportingPeriod(year, month, now, reportingWindowMonths); err != nil {
		return 0, 0, err
	}
	return year, month, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func writePeriodError(w http.ResponseWriter, err error) {
	if errors.Is(err, usage.ErrPeriodOutOfRange) {
		writeError(w, http.StatusBadRequest, "period_out_of_range")
		return
	}
	writeError(w, http.StatusBadRequest, "bad_request")
}
