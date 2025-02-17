// Copyright (c) Mainflux
// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/MainfluxLabs/mainflux/pkg/dbutil"
	"github.com/MainfluxLabs/mainflux/pkg/errors"
	"github.com/MainfluxLabs/mainflux/things"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ things.GroupRepository = (*groupRepository)(nil)

type groupRepository struct {
	db Database
}

// NewGroupRepository instantiates a PostgreSQL implementation of group
// repository.
func NewGroupRepository(db Database) things.GroupRepository {
	return &groupRepository{
		db: db,
	}
}

func (gr groupRepository) Save(ctx context.Context, g things.Group) (things.Group, error) {
	q := `INSERT INTO groups (name, description, id, org_id, metadata, created_at, updated_at)
		  VALUES (:name, :description, :id, :org_id, :metadata, :created_at, :updated_at)
		  RETURNING id, name, org_id, description, metadata, created_at, updated_at`

	dbg, err := toDBGroup(g)
	if err != nil {
		return things.Group{}, err
	}

	row, err := gr.db.NamedQueryContext(ctx, q, dbg)
	if err != nil {
		pgErr, ok := err.(*pgconn.PgError)
		if ok {
			switch pgErr.Code {
			case pgerrcode.InvalidTextRepresentation:
				return things.Group{}, errors.Wrap(errors.ErrMalformedEntity, err)
			case pgerrcode.ForeignKeyViolation:
				return things.Group{}, errors.Wrap(errors.ErrCreateEntity, err)
			case pgerrcode.UniqueViolation:
				return things.Group{}, errors.Wrap(errors.ErrConflict, err)
			case pgerrcode.StringDataRightTruncationDataException:
				return things.Group{}, errors.Wrap(errors.ErrMalformedEntity, err)
			}
		}

		return things.Group{}, errors.Wrap(errors.ErrCreateEntity, err)
	}

	defer row.Close()
	row.Next()
	dbg = dbGroup{}
	if err := row.StructScan(&dbg); err != nil {
		return things.Group{}, err
	}

	return toGroup(dbg)
}

func (gr groupRepository) Update(ctx context.Context, g things.Group) (things.Group, error) {
	q := `UPDATE groups SET name = :name, description = :description, metadata = :metadata, updated_at = :updated_at WHERE id = :id
		  RETURNING id, name, description, metadata, created_at, updated_at`

	dbu, err := toDBGroup(g)
	if err != nil {
		return things.Group{}, errors.Wrap(errors.ErrUpdateEntity, err)
	}

	row, err := gr.db.NamedQueryContext(ctx, q, dbu)
	if err != nil {
		pgErr, ok := err.(*pgconn.PgError)
		if ok {
			switch pgErr.Code {
			case pgerrcode.InvalidTextRepresentation:
				return things.Group{}, errors.Wrap(errors.ErrMalformedEntity, err)
			case pgerrcode.UniqueViolation:
				return things.Group{}, errors.Wrap(errors.ErrConflict, err)
			case pgerrcode.StringDataRightTruncationDataException:
				return things.Group{}, errors.Wrap(errors.ErrMalformedEntity, err)
			}
		}
		return things.Group{}, errors.Wrap(errors.ErrUpdateEntity, err)
	}

	defer row.Close()
	row.Next()
	dbu = dbGroup{}
	if err := row.StructScan(&dbu); err != nil {
		return g, errors.Wrap(errors.ErrUpdateEntity, err)
	}

	return toGroup(dbu)
}

func (gr groupRepository) Remove(ctx context.Context, groupIDs ...string) error {
	qd := `DELETE FROM groups WHERE id = :id`

	for _, groupID := range groupIDs {
		dbGr := dbGroup{
			ID: groupID,
		}

		res, err := gr.db.NamedExecContext(ctx, qd, dbGr)
		if err != nil {
			pqErr, ok := err.(*pgconn.PgError)
			if ok {
				switch pqErr.Code {
				case pgerrcode.InvalidTextRepresentation:
					return errors.Wrap(errors.ErrMalformedEntity, err)
				case pgerrcode.ForeignKeyViolation:
					return errors.Wrap(errors.ErrConflict, err)
				}
			}

			return errors.Wrap(errors.ErrUpdateEntity, err)
		}

		cnt, err := res.RowsAffected()
		if err != nil {
			return errors.Wrap(errors.ErrRemoveEntity, err)
		}

		if cnt != 1 {
			return errors.Wrap(errors.ErrRemoveEntity, err)
		}
	}

	return nil
}

func (gr groupRepository) RetrieveAll(ctx context.Context) ([]things.Group, error) {
	gp, err := gr.retrieve(ctx, []string{}, "", things.PageMetadata{})
	if err != nil {
		return nil, err
	}

	return gp.Groups, nil
}

func (gr groupRepository) RetrieveByID(ctx context.Context, id string) (things.Group, error) {
	dbu := dbGroup{
		ID: id,
	}
	q := `SELECT id, name, org_id, description, metadata, created_at, updated_at FROM groups WHERE id = $1`
	if err := gr.db.QueryRowxContext(ctx, q, id).StructScan(&dbu); err != nil {
		if err == sql.ErrNoRows {
			return things.Group{}, errors.Wrap(errors.ErrNotFound, err)

		}
		return things.Group{}, errors.Wrap(errors.ErrRetrieveEntity, err)
	}
	return toGroup(dbu)
}

func (gr groupRepository) RetrieveByIDs(ctx context.Context, groupIDs []string, pm things.PageMetadata) (things.GroupPage, error) {
	if len(groupIDs) == 0 {
		return things.GroupPage{}, nil
	}

	grPage, err := gr.retrieve(ctx, groupIDs, "", pm)
	if err != nil {
		return things.GroupPage{}, err
	}

	return grPage, nil
}

func (gr groupRepository) RetrieveByAdmin(ctx context.Context, orgID string, pm things.PageMetadata) (things.GroupPage, error) {
	return gr.retrieve(ctx, []string{}, orgID, pm)
}

func (gr groupRepository) retrieve(ctx context.Context, groupIDs []string, orgID string, pm things.PageMetadata) (things.GroupPage, error) {
	idsq := getIDsQuery(groupIDs)
	nq, name := dbutil.GetNameQuery(pm.Name)
	oq := dbutil.GetOrderQuery(pm.Order)
	dq := dbutil.GetDirQuery(pm.Dir)
	olq := dbutil.GetOffsetLimitQuery(pm.Limit)
	meta, mq, err := dbutil.GetMetadataQuery("", pm.Metadata)
	if err != nil {
		return things.GroupPage{}, errors.Wrap(errors.ErrRetrieveEntity, err)
	}

	var whereClause string
	var query []string
	if idsq != "" {
		query = append(query, idsq)
	}

	if mq != "" {
		query = append(query, mq)
	}

	if nq != "" {
		query = append(query, nq)
	}

	if orgID != "" {
		orgq := "org_id = :org_id"
		query = append(query, orgq)
	}

	if len(query) > 0 {
		whereClause = fmt.Sprintf(" WHERE %s", strings.Join(query, " AND "))
	}

	q := fmt.Sprintf(`SELECT id, name, org_id, description, metadata, created_at, updated_at FROM groups %s ORDER BY %s %s %s;`, whereClause, oq, dq, olq)

	params := map[string]interface{}{
		"org_id":   orgID,
		"limit":    pm.Limit,
		"offset":   pm.Offset,
		"name":     name,
		"metadata": meta,
	}

	rows, err := gr.db.NamedQueryContext(ctx, q, params)
	if err != nil {
		return things.GroupPage{}, errors.Wrap(errors.ErrRetrieveEntity, err)
	}
	defer rows.Close()

	items := []things.Group{}
	for rows.Next() {
		dbg := dbGroup{}
		if err := rows.StructScan(&dbg); err != nil {
			return things.GroupPage{}, errors.Wrap(errors.ErrRetrieveEntity, err)
		}
		gr, err := toGroup(dbg)
		if err != nil {
			return things.GroupPage{}, errors.Wrap(errors.ErrRetrieveEntity, err)
		}
		items = append(items, gr)
	}

	cq := fmt.Sprintf("SELECT COUNT(*) FROM groups %s;", whereClause)

	total, err := total(ctx, gr.db, cq, params)
	if err != nil {
		return things.GroupPage{}, errors.Wrap(errors.ErrRetrieveEntity, err)
	}

	page := things.GroupPage{
		Groups: items,
		PageMetadata: things.PageMetadata{
			Total:  total,
			Offset: pm.Offset,
			Limit:  pm.Limit,
			Order:  pm.Order,
			Dir:    pm.Dir,
		},
	}

	return page, nil
}

type dbGroup struct {
	ID          string    `db:"id"`
	OrgID       string    `db:"org_id"`
	Name        string    `db:"name"`
	Description string    `db:"description"`
	Metadata    dbJSONB   `db:"metadata"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

func toDBGroup(g things.Group) (dbGroup, error) {
	return dbGroup{
		ID:          g.ID,
		Name:        g.Name,
		OrgID:       g.OrgID,
		Description: g.Description,
		Metadata:    dbJSONB(g.Metadata),
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}, nil
}

func toGroup(dbu dbGroup) (things.Group, error) {
	return things.Group{
		ID:          dbu.ID,
		Name:        dbu.Name,
		OrgID:       dbu.OrgID,
		Description: dbu.Description,
		Metadata:    things.Metadata(dbu.Metadata),
		UpdatedAt:   dbu.UpdatedAt,
		CreatedAt:   dbu.CreatedAt,
	}, nil
}
