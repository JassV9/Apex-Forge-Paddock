package training

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

const programSQL = `
SELECT p.id, p.session_id, c.name, t.name, p.target, p.state, p.message, p.created_at
FROM training_program p JOIN car c ON c.id = p.car_id JOIN track t ON t.id = p.track_id`

// Get returns one program with its experiments.
func (s *Service) Get(ctx context.Context, id int64) (contracts.TrainingProgram, error) {
	program, err := scanProgram(s.database.QueryRowContext(ctx, programSQL+` WHERE p.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return program, fmt.Errorf("training program %d: %w", id, experiments.ErrNotFound)
	}
	if err != nil {
		return program, fmt.Errorf("read training program %d: %w", id, err)
	}
	return program, s.fillSteps(ctx, &program)
}

// List returns every program, newest first.
func (s *Service) List(ctx context.Context) ([]contracts.TrainingProgram, error) {
	rows, err := s.database.QueryContext(ctx, programSQL+` ORDER BY p.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list training programs: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	programs := []contracts.TrainingProgram{}
	for rows.Next() {
		program, err := scanProgram(rows)
		if err != nil {
			return nil, fmt.Errorf("read training program: %w", err)
		}
		programs = append(programs, program)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list training programs: %w", err)
	}
	for i := range programs {
		if err := s.fillSteps(ctx, &programs[i]); err != nil {
			return nil, err
		}
	}
	return programs, nil
}

type scanner interface{ Scan(dest ...any) error }

func scanProgram(row scanner) (contracts.TrainingProgram, error) {
	var p contracts.TrainingProgram
	var state string
	err := row.Scan(&p.Id, &p.SessionId, &p.CarName, &p.TrackName, &p.Target, &state, &p.Message, &p.CreatedAt)
	p.State = contracts.TrainingProgramState(state)
	p.Steps = []contracts.TrainingStep{}
	return p, err
}

// openPrograms lists the programs still running or waiting.
func openPrograms(ctx context.Context, q *sql.DB) ([]int64, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id FROM training_program WHERE state IN ('running', 'waiting') ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list open training programs: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read open training program: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// stepIDs lists a program's experiments, oldest first.
func stepIDs(ctx context.Context, q *sql.DB, programID int64) ([]int64, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT experiment_id FROM training_step WHERE program_id = $1 ORDER BY added_at, experiment_id`, programID)
	if err != nil {
		return nil, fmt.Errorf("read steps of program %d: %w", programID, err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read step of program %d: %w", programID, err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Service) fillSteps(ctx context.Context, program *contracts.TrainingProgram) error {
	ids, err := stepIDs(ctx, s.database, int64(program.Id))
	if err != nil {
		return err
	}
	for _, id := range ids {
		exp, err := s.ledger.Get(ctx, id)
		if err != nil {
			return fmt.Errorf("read experiment %d of program %d: %w", id, program.Id, err)
		}
		program.Steps = append(program.Steps, contracts.TrainingStep{
			ExperimentId: exp.Id, Parameter: exp.Parameter, Clicks: exp.Clicks, State: string(exp.State),
		})
	}
	return nil
}

// setState records where a program is; the message says why in words.
func (s *Service) setState(ctx context.Context, id int64, state contracts.TrainingProgramState, message string) error {
	_, err := s.database.ExecContext(ctx,
		`UPDATE training_program SET state = $2, message = $3, updated_at = now()
		 WHERE id = $1 AND (state <> $2 OR message <> $3) AND state IN ('running', 'waiting')`,
		id, string(state), message)
	if err != nil {
		return fmt.Errorf("record state of program %d: %w", id, err)
	}
	return nil
}
