package reference

import (
	"context"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// Every track row of one layout of a game track, newest corner map first.
const layoutRowsSQL = `
SELECT id, layout, name, corner_map
FROM track
WHERE ac_id = $1 AND layout = $2
ORDER BY corner_map_version DESC, id DESC`

// aiTracks returns the track rows whose AI passes a session is compared with: its own row,
// then the rows of its layout's variant whose corners match (Group). The note says why a
// variant row is not compared with. Nothing is written: the grouping is read-side only.
func (s *Service) aiTracks(ctx context.Context, info sessionInfo) ([]TrackRow, string, error) {
	own := []TrackRow{info.track}
	layout, ok := VariantLayout(info.track.Layout)
	if !ok {
		return own, "", nil
	}
	variants, err := s.layoutRows(ctx, info.acID, layout)
	if err != nil {
		return nil, "", err
	}
	grouped, note := Group(info.track, variants)
	return append(own, grouped...), note, nil
}

func (s *Service) layoutRows(ctx context.Context, acID, layout string) ([]TrackRow, error) {
	rows, err := s.database.QueryContext(ctx, layoutRowsSQL, acID, layout)
	if err != nil {
		return nil, fmt.Errorf("query track %s %s: %w", acID, layout, err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	var found []TrackRow
	for rows.Next() {
		var row TrackRow
		if err := rows.Scan(&row.ID, &row.Layout, &row.Name, &row.CornerMap); err != nil {
			return nil, fmt.Errorf("scan track %s %s: %w", acID, layout, err)
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read track %s %s: %w", acID, layout, err)
	}
	return found, nil
}

// aiPasses loads the AI's passes of the car on every grouped track row (never the session's
// own passes) and counts them per row.
func (s *Service) aiPasses(ctx context.Context, info sessionInfo, sessionID int64, tracks []TrackRow) ([]Pass, []contracts.ReferenceTrack, error) {
	var all []Pass
	counted := make([]contracts.ReferenceTrack, 0, len(tracks))
	for i, track := range tracks {
		passes, err := s.passes(ctx, aiPassesSQL, info.carID, track.ID, sessionID)
		if err != nil {
			return nil, nil, fmt.Errorf("AI passes for session %d on track %d: %w", sessionID, track.ID, err)
		}
		all = append(all, passes...)
		counted = append(counted, contracts.ReferenceTrack{Layout: track.Layout, Name: track.Name,
			Variant: i > 0, AiPasses: len(passes)})
	}
	return all, counted, nil
}
