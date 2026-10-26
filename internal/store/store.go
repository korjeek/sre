package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sre-incident-tracker/internal/models"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) ListServices(ctx context.Context) ([]models.Service, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, owner, description, created_at, updated_at
		FROM services
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.Service
	for rows.Next() {
		var item models.Service
		if err := rows.Scan(&item.ID, &item.Name, &item.Owner, &item.Description, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if items == nil {
		items = []models.Service{}
	}
	return items, rows.Err()
}

func (s *Store) GetService(ctx context.Context, id int64) (models.Service, error) {
	var item models.Service
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, owner, description, created_at, updated_at
		FROM services WHERE id = $1`, id).
		Scan(&item.ID, &item.Name, &item.Owner, &item.Description, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Service{}, ErrNotFound
	}
	return item, err
}

func (s *Store) CreateService(ctx context.Context, item models.Service) (models.Service, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO services (name, owner, description)
		VALUES ($1, $2, $3)
		RETURNING id, name, owner, description, created_at, updated_at`,
		item.Name, item.Owner, item.Description).
		Scan(&item.ID, &item.Name, &item.Owner, &item.Description, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Store) UpdateService(ctx context.Context, item models.Service) (models.Service, error) {
	err := s.pool.QueryRow(ctx, `
		UPDATE services
		SET name = $2, owner = $3, description = $4, updated_at = now()
		WHERE id = $1
		RETURNING id, name, owner, description, created_at, updated_at`,
		item.ID, item.Name, item.Owner, item.Description).
		Scan(&item.ID, &item.Name, &item.Owner, &item.Description, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Service{}, ErrNotFound
	}
	return item, err
}

func (s *Store) DeleteService(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM services WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListIncidents(ctx context.Context) ([]models.Incident, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, service_id, title, description, severity, status, created_at, updated_at
		FROM incidents
		ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.Incident
	for rows.Next() {
		var item models.Incident
		if err := rows.Scan(
			&item.ID, &item.ServiceID, &item.Title, &item.Description,
			&item.Severity, &item.Status, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if items == nil {
		items = []models.Incident{}
	}
	return items, rows.Err()
}

func (s *Store) GetIncident(ctx context.Context, id int64) (models.Incident, error) {
	var item models.Incident
	err := s.pool.QueryRow(ctx, `
		SELECT id, service_id, title, description, severity, status, created_at, updated_at
		FROM incidents WHERE id = $1`, id).
		Scan(&item.ID, &item.ServiceID, &item.Title, &item.Description, &item.Severity, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Incident{}, ErrNotFound
	}
	return item, err
}

func (s *Store) CreateIncident(ctx context.Context, item models.Incident) (models.Incident, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO incidents (service_id, title, description, severity, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, service_id, title, description, severity, status, created_at, updated_at`,
		item.ServiceID, item.Title, item.Description, item.Severity, item.Status).
		Scan(&item.ID, &item.ServiceID, &item.Title, &item.Description, &item.Severity, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return models.Incident{}, fmt.Errorf("create incident: %w", err)
	}
	return item, nil
}

func (s *Store) UpdateIncident(ctx context.Context, item models.Incident) (models.Incident, error) {
	err := s.pool.QueryRow(ctx, `
		UPDATE incidents
		SET service_id = $2, title = $3, description = $4, severity = $5, status = $6, updated_at = now()
		WHERE id = $1
		RETURNING id, service_id, title, description, severity, status, created_at, updated_at`,
		item.ID, item.ServiceID, item.Title, item.Description, item.Severity, item.Status).
		Scan(&item.ID, &item.ServiceID, &item.Title, &item.Description, &item.Severity, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Incident{}, ErrNotFound
	}
	return item, err
}

func (s *Store) DeleteIncident(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM incidents WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) IncidentCounts(ctx context.Context) ([]models.IncidentCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT status, severity, count(*)
		FROM incidents
		GROUP BY status, severity`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.IncidentCount
	for rows.Next() {
		var item models.IncidentCount
		if err := rows.Scan(&item.Status, &item.Severity, &item.Count); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ServicesCount(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM services`).Scan(&n)
	return n, err
}
