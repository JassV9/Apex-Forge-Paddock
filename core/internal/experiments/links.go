package experiments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/setups"
)

// The setups the agent read back from the car: a version gets the agent's fingerprint only
// when it was on the car, so a variant that never got there (it may not exist on this car)
// does not count.
const recordedVersionsSQL = `
SELECT "values" FROM setup_version WHERE car_id = $1 AND agent_hash IS NOT NULL ORDER BY id`

// loadLinks is the car's linked setup items (docs/adr/0026-linked-setup-items.md), from the
// setups recorded on it and the groups declared for it.
func loadLinks(ctx context.Context, q querier, carID int64, catalog setups.Catalog) (setups.Links, error) {
	var gameID string
	if err := q.QueryRowContext(ctx, `SELECT ac_id FROM car WHERE id = $1`, carID).Scan(&gameID); err != nil {
		return nil, fmt.Errorf("read game id of car %d: %w", carID, err)
	}
	recorded, err := queryAll(ctx, q, recordedVersionsSQL, func(rows *sql.Rows) (setups.Values, error) {
		var encoded []byte
		values := setups.Values{}
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		return values, json.Unmarshal(encoded, &values)
	}, carID)
	if err != nil {
		return nil, fmt.Errorf("read recorded setups of car %d: %w", carID, err)
	}
	return setups.DetectLinks(catalog, recorded, setups.DeclaredLinks[gameID]), nil
}

// Menu is a car's setup menu with its linked items.
type Menu struct {
	Catalog setups.Catalog
	Links   setups.Links
}

// CarMenu reads the car's setup menu and which of its items are linked, for the ranking and
// the training program: a group is one candidate there as it is one change here.
func CarMenu(ctx context.Context, database *sql.DB, carID int64) (Menu, error) {
	catalog, _, err := loadCatalog(ctx, database, carID)
	if err != nil {
		return Menu{}, err
	}
	links, err := loadLinks(ctx, database, carID, catalog)
	if err != nil {
		return Menu{}, err
	}
	return Menu{Catalog: catalog, Links: links}, nil
}

// SessionMenu is CarMenu for the session's car.
func SessionMenu(ctx context.Context, database *sql.DB, sessionID int64) (Menu, error) {
	var carID int64
	err := database.QueryRowContext(ctx, `SELECT car_id FROM session WHERE id = $1`, sessionID).Scan(&carID)
	if errors.Is(err, sql.ErrNoRows) {
		return Menu{}, fmt.Errorf("session %d: %w", sessionID, ErrNotFound)
	}
	if err != nil {
		return Menu{}, fmt.Errorf("find car of session %d: %w", sessionID, err)
	}
	return CarMenu(ctx, database, carID)
}
