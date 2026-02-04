package services

import (
	"database/sql"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
)


type TransactionFunc func(*sql.Tx) (interface{}, error)


type BaseService struct {
	db     *sql.DB
	logger *utils.Logger
}


func NewBaseService(db *sql.DB, logger *utils.Logger) BaseService {
	return BaseService{
		db:     db,
		logger: logger,
	}
}


func (s *BaseService) ExecuteInTransaction(fn TransactionFunc) (interface{}, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, utils.NewInternalServerError("Failed to begin transaction", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	result, err := fn(tx)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, utils.NewInternalServerError("Failed to commit transaction", err)
	}

	return result, nil
}


func (s *BaseService) LogDebug(format string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Debug(format, args...)
	}
}


func (s *BaseService) LogInfo(format string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Info(format, args...)
	}
}


func (s *BaseService) LogWarning(format string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Warning(format, args...)
	}
}


func (s *BaseService) LogError(err error, format string, args ...interface{}) {
	if s.logger != nil {
		s.logger.LogWithError(utils.ERROR, err, format, args...)
	}
}
