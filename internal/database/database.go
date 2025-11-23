package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand/v2"
	"time"

	"pr-review-service/internal/models"

	_ "github.com/lib/pq"
)

type DB struct {
	db *sql.DB
}

func New(databaseURL string) (*DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	log.Println("Database connection established")
	return &DB{db: db}, nil
}

func (db *DB) Close() {
	db.db.Close()
}

func (db *DB) CreateTeam(ctx context.Context, team *models.Team) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM teams WHERE team_name = $1)", team.TeamName).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf(models.ErrTeamExists)
	}

	_, err = tx.ExecContext(ctx, "INSERT INTO teams (team_name) VALUES ($1)", team.TeamName)
	if err != nil {
		return err
	}

	for _, member := range team.Members {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO users (user_id, username, team_name, is_active)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_id) DO UPDATE
			SET username = EXCLUDED.username,
			    team_name = EXCLUDED.team_name,
			    is_active = EXCLUDED.is_active
		`, member.UserID, member.Username, team.TeamName, member.IsActive)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) GetTeam(ctx context.Context, teamName string) (*models.Team, error) {
	var exists bool
	err := db.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM teams WHERE team_name = $1)", teamName).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf(models.ErrNotFound)
	}

	rows, err := db.db.QueryContext(ctx, `
		SELECT user_id, username, is_active
		FROM users
		WHERE team_name = $1
		ORDER BY username
	`, teamName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := []models.TeamMember{}
	for rows.Next() {
		var member models.TeamMember
		if err := rows.Scan(&member.UserID, &member.Username, &member.IsActive); err != nil {
			return nil, err
		}
		members = append(members, member)
	}

	return &models.Team{
		TeamName: teamName,
		Members:  members,
	}, nil
}

func (db *DB) SetUserActive(ctx context.Context, userID string, isActive bool) (*models.User, error) {
	var user models.User
	err := db.db.QueryRowContext(ctx, `
		UPDATE users
		SET is_active = $2
		WHERE user_id = $1
		RETURNING user_id, username, team_name, is_active
	`, userID, isActive).Scan(&user.UserID, &user.Username, &user.TeamName, &user.IsActive)

	if err != nil {
		return nil, fmt.Errorf(models.ErrNotFound)
	}

	return &user, nil
}

func (db *DB) CreatePR(ctx context.Context, prID, prName, authorID string) (*models.PullRequest, error) {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pull_requests WHERE pull_request_id = $1)", prID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf(models.ErrPRExists)
	}

	var teamName string
	err = tx.QueryRowContext(ctx, "SELECT team_name FROM users WHERE user_id = $1", authorID).Scan(&teamName)
	if err != nil {
		return nil, fmt.Errorf(models.ErrNotFound)
	}

	now := time.Now()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO pull_requests (pull_request_id, pull_request_name, author_id, status, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, prID, prName, authorID, models.StatusOpen, now)
	if err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT user_id FROM users
		WHERE team_name = $1 AND is_active = true AND user_id != $2
	`, teamName, authorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := []string{}
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		candidates = append(candidates, userID)
	}

	reviewers := selectRandomReviewers(candidates, 2)
	for _, reviewerID := range reviewers {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO pr_reviewers (pull_request_id, user_id)
			VALUES ($1, $2)
		`, prID, reviewerID)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &models.PullRequest{
		PullRequestID:     prID,
		PullRequestName:   prName,
		AuthorID:          authorID,
		Status:            models.StatusOpen,
		AssignedReviewers: reviewers,
		CreatedAt:         &now,
	}, nil
}

func (db *DB) MergePR(ctx context.Context, prID string) (*models.PullRequest, error) {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var pr models.PullRequest
	var mergedAt *time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT pull_request_id, pull_request_name, author_id, status, created_at, merged_at
		FROM pull_requests
		WHERE pull_request_id = $1
	`, prID).Scan(&pr.PullRequestID, &pr.PullRequestName, &pr.AuthorID, &pr.Status, &pr.CreatedAt, &mergedAt)

	if err != nil {
		return nil, fmt.Errorf(models.ErrNotFound)
	}

	if pr.Status == models.StatusMerged {
		pr.MergedAt = mergedAt
		rows, _ := tx.QueryContext(ctx, `SELECT user_id FROM pr_reviewers WHERE pull_request_id = $1`, prID)
		reviewers := []string{}
		for rows.Next() {
			var userID string
			if rows.Scan(&userID) == nil {
				reviewers = append(reviewers, userID)
			}
		}
		rows.Close()
		pr.AssignedReviewers = reviewers
		return &pr, nil
	}

	now := time.Now()
	_, err = tx.ExecContext(ctx, `
		UPDATE pull_requests
		SET status = $2, merged_at = $3
		WHERE pull_request_id = $1
	`, prID, models.StatusMerged, now)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	pr.Status = models.StatusMerged
	pr.MergedAt = &now
	pr.AssignedReviewers = db.getReviewersFromDB(ctx, prID)

	return &pr, nil
}

func (db *DB) ReassignReviewer(ctx context.Context, prID, oldUserID string) (*models.PullRequest, string, error) {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRowContext(ctx, "SELECT status FROM pull_requests WHERE pull_request_id = $1", prID).Scan(&status)
	if err != nil {
		return nil, "", fmt.Errorf(models.ErrNotFound)
	}

	if status == models.StatusMerged {
		return nil, "", fmt.Errorf(models.ErrPRMerged)
	}

	var isAssigned bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM pr_reviewers WHERE pull_request_id = $1 AND user_id = $2)
	`, prID, oldUserID).Scan(&isAssigned)
	if err != nil {
		return nil, "", err
	}
	if !isAssigned {
		return nil, "", fmt.Errorf(models.ErrNotAssigned)
	}

	var teamName, authorID string
	err = tx.QueryRowContext(ctx, `
		SELECT u.team_name, pr.author_id
		FROM users u, pull_requests pr
		WHERE u.user_id = $1 AND pr.pull_request_id = $2
	`, oldUserID, prID).Scan(&teamName, &authorID)
	if err != nil {
		return nil, "", err
	}

	rowsCurr, _ := tx.QueryContext(ctx, `SELECT user_id FROM pr_reviewers WHERE pull_request_id = $1`, prID)
	currentReviewers := []string{}
	for rowsCurr.Next() {
		var userID string
		if rowsCurr.Scan(&userID) == nil {
			currentReviewers = append(currentReviewers, userID)
		}
	}
	rowsCurr.Close()

	rows, err := tx.QueryContext(ctx, `
		SELECT user_id FROM users
		WHERE team_name = $1 AND is_active = true AND user_id != $2
	`, teamName, authorID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	candidates := []string{}
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, "", err
		}
		isCurrentReviewer := false
		for _, r := range currentReviewers {
			if r == userID {
				isCurrentReviewer = true
				break
			}
		}
		if !isCurrentReviewer {
			candidates = append(candidates, userID)
		}
	}

	if len(candidates) == 0 {
		return nil, "", fmt.Errorf(models.ErrNoCandidate)
	}

	newReviewer := candidates[rand.IntN(len(candidates))]

	_, err = tx.ExecContext(ctx, `
		DELETE FROM pr_reviewers WHERE pull_request_id = $1 AND user_id = $2
	`, prID, oldUserID)
	if err != nil {
		return nil, "", err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO pr_reviewers (pull_request_id, user_id)
		VALUES ($1, $2)
	`, prID, newReviewer)
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(); err != nil {
		return nil, "", err
	}

	pr, err := db.GetPR(ctx, prID)
	if err != nil {
		return nil, "", err
	}

	return pr, newReviewer, nil
}

func (db *DB) GetUserReviews(ctx context.Context, userID string) ([]models.PullRequestShort, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT pr.pull_request_id, pr.pull_request_name, pr.author_id, pr.status
		FROM pull_requests pr
		JOIN pr_reviewers r ON pr.pull_request_id = r.pull_request_id
		WHERE r.user_id = $1
		ORDER BY pr.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prs := []models.PullRequestShort{}
	for rows.Next() {
		var pr models.PullRequestShort
		if err := rows.Scan(&pr.PullRequestID, &pr.PullRequestName, &pr.AuthorID, &pr.Status); err != nil {
			return nil, err
		}
		prs = append(prs, pr)
	}

	return prs, nil
}

func (db *DB) GetPR(ctx context.Context, prID string) (*models.PullRequest, error) {
	var pr models.PullRequest
	err := db.db.QueryRowContext(ctx, `
		SELECT pull_request_id, pull_request_name, author_id, status, created_at, merged_at
		FROM pull_requests
		WHERE pull_request_id = $1
	`, prID).Scan(&pr.PullRequestID, &pr.PullRequestName, &pr.AuthorID, &pr.Status, &pr.CreatedAt, &pr.MergedAt)

	if err != nil {
		return nil, fmt.Errorf(models.ErrNotFound)
	}

	pr.AssignedReviewers = db.getReviewersFromDB(ctx, prID)
	return &pr, nil
}

func (db *DB) getReviewersFromDB(ctx context.Context, prID string) []string {
	rows, err := db.db.QueryContext(ctx, `
		SELECT user_id FROM pr_reviewers WHERE pull_request_id = $1
	`, prID)
	if err != nil {
		return []string{}
	}
	defer rows.Close()

	reviewers := []string{}
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err == nil {
			reviewers = append(reviewers, userID)
		}
	}
	return reviewers
}

func selectRandomReviewers(candidates []string, max int) []string {
	if len(candidates) <= max {
		return candidates
	}

	selected := make([]string, max)
	perm := rand.Perm(len(candidates))
	for i := 0; i < max; i++ {
		selected[i] = candidates[perm[i]]
	}
	return selected
}
