package user

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("user: not found")

// Repository adalah kontrak data access untuk domain user. Business rule
// (misal email unique message yang jelas) tetap ditangani service layer,
// repository hanya bicara ke database.
type Repository interface {
	Create(ctx context.Context, u *User) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context, filter ListFilter) ([]User, int64, error)
	Update(ctx context.Context, u *User) error
}

type ListFilter struct {
	Search string
	Role   string
	Status string
	Page   int
	Limit  int
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, u *User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *gormRepository) FindByID(ctx context.Context, id string) (*User, error) {
	var u User
	if err := r.db.WithContext(ctx).First(&u, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *gormRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	if err := r.db.WithContext(ctx).First(&u, "email = ?", email).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *gormRepository) List(ctx context.Context, f ListFilter) ([]User, int64, error) {
	q := r.db.WithContext(ctx).Model(&User{})

	if f.Search != "" {
		q = q.Where("name ILIKE ? OR email ILIKE ?", "%"+f.Search+"%", "%"+f.Search+"%")
	}
	if f.Role != "" {
		q = q.Where("role = ?", f.Role)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []User
	offset := (f.Page - 1) * f.Limit
	if err := q.Order("created_at desc").Offset(offset).Limit(f.Limit).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func (r *gormRepository) Update(ctx context.Context, u *User) error {
	return r.db.WithContext(ctx).Save(u).Error
}
