package category

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("category: not found")

type Repository interface {
	Create(ctx context.Context, c *Category) error
	FindByID(ctx context.Context, id string) (*Category, error)
	FindByName(ctx context.Context, name string) (*Category, error)
	List(ctx context.Context, search string, page, limit int) ([]Category, int64, error)
	Update(ctx context.Context, c *Category) error
	Delete(ctx context.Context, id string) error
	// HasActiveProducts mengecek apakah kategori masih punya produk dengan
	// status ACTIVE — dipakai service untuk mencegah delete sembarangan
	// (spec Section 28). Query lintas tabel (products) sengaja ditaruh di
	// sini (repository layer) via raw join, bukan di service, agar service
	// tidak perlu tahu detail skema tabel products.
	HasActiveProducts(ctx context.Context, categoryID string) (bool, error)
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, c *Category) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *gormRepository) FindByID(ctx context.Context, id string) (*Category, error) {
	var c Category
	if err := r.db.WithContext(ctx).First(&c, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (r *gormRepository) FindByName(ctx context.Context, name string) (*Category, error) {
	var c Category
	if err := r.db.WithContext(ctx).First(&c, "name = ?", name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (r *gormRepository) List(ctx context.Context, search string, page, limit int) ([]Category, int64, error) {
	q := r.db.WithContext(ctx).Model(&Category{})
	if search != "" {
		q = q.Where("name ILIKE ?", "%"+search+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []Category
	offset := (page - 1) * limit
	if err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *gormRepository) Update(ctx context.Context, c *Category) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *gormRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&Category{}, "id = ?", id).Error
}

func (r *gormRepository) HasActiveProducts(ctx context.Context, categoryID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("products").
		Where("category_id = ? AND status = ? AND deleted_at IS NULL", categoryID, "ACTIVE").
		Count(&count).Error
	return count > 0, err
}
