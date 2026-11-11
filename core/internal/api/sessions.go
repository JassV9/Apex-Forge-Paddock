package api

import (
	"log/slog"
	"net/http"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// sessionsHandler answers GET /sessions.
func sessionsHandler(logger *slog.Logger, sessions SessionLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := sessions.ListSessions(r.Context())
		if err != nil {
			writeError(w, r, logger, http.StatusInternalServerError, "could not list sessions", err)
			return
		}
		if list == nil {
			list = []contracts.SessionSummary{} // an empty list is [], never null
		}
		writeJSON(w, r, logger, http.StatusOK, list)
	}
}
