package model

import "time"

type Article struct {
	ID             string `gorm:"primaryKey"`
	Slug           string `gorm:"not null;unique"`
	Title          string `gorm:"not null"`
	Summary        string `gorm:"not null"`
	Category       string
	Tags           []string `gorm:"serializer:json;type:json"`
	Status         string
	PublishedAt    *time.Time
	ReadingMinutes int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
