package category

type CreateRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

type UpdateRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type Response struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func toResponse(c *Category) Response {
	return Response{ID: c.ID, Name: c.Name, Description: c.Description}
}
