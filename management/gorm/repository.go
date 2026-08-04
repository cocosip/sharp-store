package gorm

import (
	"context"
	"encoding/json"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/management"
	gormio "gorm.io/gorm"
)

const DefaultTableName = "store_containers"

type ContainerModel struct {
	ID         string `gorm:"primaryKey;size:64"`
	TenantID   string `gorm:"not null;size:128;uniqueIndex:idx_store_container_tenant_key"`
	Key        string `gorm:"column:container_key;not null;size:255;uniqueIndex:idx_store_container_tenant_key"`
	Title      string `gorm:"not null;size:255"`
	Backend    string `gorm:"not null;size:128"`
	TenantMode uint8  `gorm:"not null"`
	Values     []byte `gorm:"not null"`
	Version    uint64 `gorm:"not null"`
}

type Repository struct {
	db    *gormio.DB
	table string
}

type Options struct {
	Table string
}

// TableName returns the table selected by options. Applications use it when
// migrating ContainerModel with their own GORM lifecycle.
func TableName(options ...Options) string {
	table := DefaultTableName
	if len(options) != 0 && options[0].Table != "" {
		table = options[0].Table
	}
	return table
}

// New creates a configuration repository. Schema migration remains owned by
// the application; use TableName with ContainerModel before using New.
func New(db *gormio.DB, options ...Options) management.Repository {
	return &Repository{db: db, table: TableName(options...)}
}

func (r *Repository) Create(ctx context.Context, container management.Container) error {
	model, err := toModel(container)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Table(r.table).Create(&model).Error
}

func (r *Repository) Get(ctx context.Context, id string) (management.Container, bool, error) {
	var model ContainerModel
	err := r.db.WithContext(ctx).Table(r.table).First(&model, "id = ?", id).Error
	if err != nil {
		if err == gormio.ErrRecordNotFound {
			return management.Container{}, false, nil
		}
		return management.Container{}, false, err
	}
	container, err := fromModel(model)
	if err != nil {
		return management.Container{}, false, err
	}
	return container, true, nil
}

func (r *Repository) Update(ctx context.Context, container management.Container) error {
	model, err := toModel(container)
	if err != nil {
		return err
	}
	result := r.db.WithContext(ctx).Table(r.table).Model(&ContainerModel{}).
		Where("id = ?", model.ID).
		Updates(map[string]any{
			"tenant_id":     model.TenantID,
			"container_key": model.Key,
			"title":         model.Title,
			"backend":       model.Backend,
			"tenant_mode":   model.TenantMode,
			"values":        model.Values,
			"version":       model.Version,
		})
	return result.Error
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Table(r.table).Delete(&ContainerModel{}, "id = ?", id).Error
}

func (r *Repository) Find(
	ctx context.Context,
	key store.ContainerKey,
	scope store.Scope,
) (management.Container, bool, error) {
	var model ContainerModel
	err := r.db.WithContext(ctx).Table(r.table).
		Where("tenant_id = ? AND container_key = ?", scope.Tenant.ID, string(key)).
		First(&model).Error
	if err != nil {
		if err == gormio.ErrRecordNotFound {
			return management.Container{}, false, nil
		}
		return management.Container{}, false, err
	}
	container, err := fromModel(model)
	if err != nil {
		return management.Container{}, false, err
	}
	return container, true, nil
}

func toModel(container management.Container) (ContainerModel, error) {
	values, err := json.Marshal(container.Config.Values)
	if err != nil {
		return ContainerModel{}, err
	}
	return ContainerModel{
		ID:         container.ID,
		TenantID:   container.TenantID,
		Key:        string(container.Key),
		Title:      container.Title,
		Backend:    container.Config.Backend,
		TenantMode: uint8(container.Config.TenantMode),
		Values:     values,
		Version:    container.Version,
	}, nil
}

func fromModel(model ContainerModel) (management.Container, error) {
	values := make(map[string]string)
	if err := json.Unmarshal(model.Values, &values); err != nil {
		return management.Container{}, err
	}
	return management.Container{
		ID:       model.ID,
		TenantID: model.TenantID,
		Key:      store.ContainerKey(model.Key),
		Title:    model.Title,
		Version:  model.Version,
		Config: store.ContainerConfig{
			Backend:    model.Backend,
			TenantMode: store.TenantMode(model.TenantMode),
			Values:     values,
		},
	}, nil
}
