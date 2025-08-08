package models

import (
	"time"
)

type User struct {
	ID        string    `bun:"id,pk"`
	Email     string    `bun:"email,unique,notnull"`
	FullName  string    `bun:"full_name,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull"`
}
