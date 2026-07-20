package web

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/ryabkov82/vpnbot/internal/config"
	"github.com/ryabkov82/vpnbot/internal/models"
	"github.com/ryabkov82/vpnbot/internal/webuser"
)

// adminAccountTestApp — поиск web-пользователя и услуг (stub в тестах).
type adminAccountTestApp interface {
	GetUserByLogin(login string) (*models.User, error)
	GetUserServicesByUserID(userID int) ([]models.UserService, error)
}

type adminAccountTestRequestJSON struct {
	Email string `json:"email"`
}

type adminAccountTestUserJSON struct {
	UserID  int     `json:"user_id"`
	Login   string  `json:"login"`
	Email   string  `json:"email"`
	Balance float64 `json:"balance"`
}

type adminAccountTestServiceJSON struct {
	UserServiceID int    `json:"user_service_id"`
	ServiceID     int    `json:"service_id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	Expire        string `json:"expire"`
	Period        string `json:"period"`
	Category      string `json:"category"`
}

type adminAccountTestOKJSON struct {
	User     adminAccountTestUserJSON      `json:"user"`
	Services []adminAccountTestServiceJSON `json:"services"`
}

func serveAdminAccountTest(cfg *config.Config, app adminAccountTestApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/account/test" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}

		wantTok := ""
		if cfg != nil {
			wantTok = cfg.Admin.Token
		}
		if !adminTokenMatches(wantTok, r.Header.Get("X-Admin-Token")) {
			writeJSONError(w, http.StatusForbidden, "forbidden")
			return
		}

		const maxBody = 1 << 20
		dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
		var req adminAccountTestRequestJSON
		if err := dec.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_request")
			return
		}

		emailNorm, err := webuser.NormalizeEmail(req.Email)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_email")
			return
		}

		login := webuser.WebLoginFromEmailWithPrefix(emailNorm, cfg.WebUserLoginPrefix())
		user, err := app.GetUserByLogin(login)
		if err != nil {
			slog.Error("admin account test: GetUserByLogin", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if user == nil {
			writeJSONError(w, http.StatusNotFound, "user_not_found")
			return
		}

		services, err := app.GetUserServicesByUserID(user.ID)
		if err != nil {
			slog.Error("admin account test: GetUserServicesByUserID", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "services_failed")
			return
		}

		outSvcs := make([]adminAccountTestServiceJSON, 0, len(services))
		for i := range services {
			us := &services[i]
			outSvcs = append(outSvcs, adminAccountTestServiceJSON{
				UserServiceID: us.ServiceID,
				ServiceID:     us.BaseServiceID,
				Name:          us.Name,
				Status:        us.Status,
				Expire:        us.Expire,
				Period:        us.Period,
				Category:      us.Category,
			})
		}

		writeJSON(w, http.StatusOK, adminAccountTestOKJSON{
			User: adminAccountTestUserJSON{
				UserID:  user.ID,
				Login:   user.Login,
				Email:   emailNorm,
				Balance: user.Balance,
			},
			Services: outSvcs,
		})
	}
}
