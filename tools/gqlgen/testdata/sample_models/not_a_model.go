package models

// NotAModel deliberately omits GraphQLMixin — should not be picked up
// by ParseModels.
type NotAModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
