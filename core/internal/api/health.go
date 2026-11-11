package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

const databasePingTimeout = 2 * time.Second

// healthHandler answers GET /health. The process is "degraded", not down, when the database
// cannot be reached: it is still able to answer, and the caller can show why it is limited.
func healthHandler(logger *slog.Logger, version string, database Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reply := contracts.Health{
			Service: contracts.HealthServiceCore,
			Version: version,
			Status:  contracts.HealthStatusOk,
		}
		ctx, cancel := context.WithTimeout(r.Context(), databasePingTimeout)
		defer cancel()
		if err := database.PingContext(ctx); err != nil {
			logger.WarnContext(ctx, "database ping failed", "error", err)
			reply.Status = contracts.HealthStatusDegraded
		}
		writeJSON(w, r, logger, http.StatusOK, reply)
	}
}
