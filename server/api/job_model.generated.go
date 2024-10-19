// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	CreateJobTableSQL = `
    CREATE TABLE jobs (
        id VARCHAR(26) NOT NULL PRIMARY KEY,
	fingerprint VARCHAR(8) NOT NULL UNIQUE KEY,
	type VARCHAR(50) NOT NULL,
	payload JSON NOT NULL,
	retries TINYINT(3) NOT NULL DEFAULT 0,
	max_retries TINYINT(3) NOT NULL DEFAULT 10,
	timeout_seconds INT(11) NOT NULL DEFAULT 60,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	scheduled_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP KEY,
	claimed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
    ) DEFAULT CHARSET=utf8mb4 ;`

	createJobSQL = `INSERT INTO jobs (id, fingerprint, type, payload) VALUES (?, ?, ?, ?);`

	getJobSQL = `SELECT * FROM jobs WHERE id=?;`

	getJobKeySQL = `SELECT  FROM jobs WHERE id=? AND fingerprint=? AND type=? AND payload=?;`

	listJobSQL = `SELECT * FROM jobs;`

	updateJobSQL = `UPDATE jobs SET fingerprint=?, type=?, payload=?, retries=?, max_retries=?, timeout_seconds=?, created_at=?, scheduled_at=?, claimed_at=? WHERE id=?;`

	deleteJobSQL = `DELETE FROM jobs WHERE id=?;`
)

type Job struct {
	Id             string    `json:"id" uri:"id"`
	Fingerprint    string    `json:"fingerprint" uri:"fingerprint" form:"fingerprint"`
	Type           string    `json:"type" uri:"type" form:"type"`
	Payload        string    `json:"payload" uri:"payload" form:"payload"`
	Retries        uint8     `json:"retries" uri:"retries" form:"retries"`
	MaxRetries     uint8     `json:"max_retries" uri:"max_retries" form:"max_retries"`
	TimeoutSeconds uint16    `json:"timeout_seconds" uri:"timeout_seconds" form:"timeout_seconds"`
	CreatedAt      time.Time `json:"created_at" uri:"created_at" form:"created_at"`
	ScheduledAt    time.Time `json:"scheduled_at" uri:"scheduled_at" form:"scheduled_at"`
	ClaimedAt      time.Time `json:"claimed_at" uri:"claimed_at" form:"claimed_at"`
}

func (model Job) String() string {
	return fmt.Sprintf("Id: %v, Fingerprint: %v, Type: %v, Payload: %v, Retries: %v, MaxRetries: %v, TimeoutSeconds: %v, CreatedAt: %v, ScheduledAt: %v, ClaimedAt: %v", model.Id, model.Fingerprint, model.Type, model.Payload, model.Retries, model.MaxRetries, model.TimeoutSeconds, model.CreatedAt, model.ScheduledAt, model.ClaimedAt)
}

type JobManager struct {
	DB *sql.DB
}

func (m *JobManager) Create(model *Job) (int, string, error) {
	status := http.StatusCreated
	_, err := m.DB.Exec(createJobSQL, model.Id, model.Fingerprint, model.Type, model.Payload)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add job to database"
			return http.StatusInternalServerError, msg, err
		}

		return http.StatusOK, "", nil
	}

	return status, "", nil
}

func (m *JobManager) Get(id string) (*Job, int, string, error) {
	model := &Job{}
	err := m.DB.QueryRow(getJobSQL, id).Scan(&model.Id, &model.Fingerprint, &model.Type, &model.Payload, &model.Retries, &model.MaxRetries, &model.TimeoutSeconds, &model.CreatedAt, &model.ScheduledAt, &model.ClaimedAt)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a job with that id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get job from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *JobManager) List() (*[]Job, int, string, error) {
	models := []Job{}
	rows, err := m.DB.Query(listJobSQL)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get jobs from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Job{}
		err = rows.Scan(&model.Id, &model.Fingerprint, &model.Type, &model.Payload, &model.Retries, &model.MaxRetries, &model.TimeoutSeconds, &model.CreatedAt, &model.ScheduledAt, &model.ClaimedAt)
		if err != nil {
			msg := "Couldn't scan row from database"
			return nil, http.StatusInternalServerError, msg, err
		}
		models = append(models, model)
	}
	err = rows.Err()
	if err != nil {
		msg := "Error scanning rows from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return &models, http.StatusOK, "", nil
}

func (m *JobManager) CustomList(sql string) (*[]Job, int, string, error) {
	models := []Job{}
	rows, err := m.DB.Query(sql)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get jobs from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Job{}
		err = rows.Scan(&model.Id, &model.Fingerprint, &model.Type, &model.Payload, &model.Retries, &model.MaxRetries, &model.TimeoutSeconds, &model.CreatedAt, &model.ScheduledAt, &model.ClaimedAt)
		if err != nil {
			msg := "Couldn't scan row from database"
			return nil, http.StatusInternalServerError, msg, err
		}
		models = append(models, model)
	}
	err = rows.Err()
	if err != nil {
		msg := "Error scanning rows from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return &models, http.StatusOK, "", nil
}

func (m *JobManager) CustomSql(sql string) (int, string, error) {
	_, err := m.DB.Query(sql)
	if err != nil {
		msg := "Couldn't run sql for Job in database"
		return http.StatusBadRequest, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *JobManager) Update(model *Job) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.Id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateJobSQL, model.Fingerprint, model.Type, model.Payload, model.Retries, model.MaxRetries, model.TimeoutSeconds, model.CreatedAt, model.ScheduledAt, model.ClaimedAt, model.Id)
	if err != nil {
		msg := "Couldn't update job in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *JobManager) Delete(id string) (int, string, error) {
	result, err := m.DB.Exec(deleteJobSQL, id)
	if err != nil {
		msg := "Couldn't delete job in database"
		return http.StatusInternalServerError, msg, err
	}
	// Check for 404s
	// Ignore errors (if the database doesn't support RowsAffected)
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return http.StatusNotFound, "", nil
	}
	return http.StatusNoContent, "", nil
}
