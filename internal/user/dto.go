package user

type CreateRequest struct {
	Name     string `json:"name" validate:"required"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	Role     string `json:"role" validate:"required,oneof=ADMIN SALES WAREHOUSE CUSTOMER"`
}

type UpdateRequest struct {
	Name  *string `json:"name"`
	Email *string `json:"email" validate:"omitempty,email"`
	Role  *string `json:"role" validate:"omitempty,oneof=ADMIN SALES WAREHOUSE CUSTOMER"`
}

type UpdateStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=ACTIVE INACTIVE SUSPENDED"`
}

type Response struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

func toResponse(u *User) Response {
	return Response{
		ID:     u.ID,
		Name:   u.Name,
		Email:  u.Email,
		Role:   string(u.Role),
		Status: string(u.Status),
	}
}
