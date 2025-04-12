package db

import (
	"gtorrent/db/models"
)

// UpdateDownload updates a download record in the database
func (d *Database) UpdateDownload(download *models.Download) error {
	return d.db.Save(download).Error
}

// UpdatePiece updates a piece record in the database
func (d *Database) UpdatePiece(piece *models.Piece) error {
	return d.db.Save(piece).Error
}
