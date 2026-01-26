package service

import (
	"github.com/Nomadcxx/jellywatch/internal/database"
)

// CleanupService provides operations for cleaning up media libraries
type CleanupService struct {
	db *database.MediaDB
}

// NewCleanupService creates a new cleanup service
func NewCleanupService(db *database.MediaDB) *CleanupService {
	return &CleanupService{db: db}
}
