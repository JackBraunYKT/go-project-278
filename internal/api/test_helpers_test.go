package api

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	repository "github.com/JackBraunYKT/go-project-278/internal/repository"
)

type LinkFixture struct {
	ID          int64  `yaml:"id"`
	OriginalURL string `yaml:"original_url"`
	ShortName   string `yaml:"short_name"`
	ShortURL    string `yaml:"short_url"`
}

type LinkFixtures struct {
	Links []LinkFixture `yaml:"links"`
}

type LinkVisitFixture struct {
	ID        int64  `yaml:"id"`
	ShortName string `yaml:"short_name"`
	CreatedAt string `yaml:"created_at"`
	IP        string `yaml:"ip"`
	UserAgent string `yaml:"user_agent"`
	Referer   string `yaml:"referer"`
	Status    int32  `yaml:"status"`
}

type LinkVisitFixtures struct {
	LinkVisits []LinkVisitFixture `yaml:"link_visits"`
}

var testPool *pgxpool.Pool

// TestMain подготавливает тестовую базу данных и запускает тесты пакета.
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/link_shortener_test?sslmode=disable"
	}

	sqlDB, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}

	if err := goose.Up(sqlDB, migrationsPath()); err != nil {
		log.Fatal(err)
	}

	testPool, err = pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatal(err)
	}

	code := m.Run()
	testPool.Close()
	os.Exit(code)
}

func setupTx(t *testing.T) (pgx.Tx, *repository.Queries) {
	tx, err := testPool.Begin(context.Background())
	require.NoError(t, err)

	q := repository.New(tx)

	t.Cleanup(func() {
		err := tx.Rollback(context.Background())
		require.NoError(t, err)
	})

	return tx, q
}

// LoadLinkFixtures загружает фикстуры ссылок из testdata.
func LoadLinkFixtures(t *testing.T) ([]LinkFixture, error) {
	fixturesDir := fixturesPath(t)
	path := filepath.Clean(filepath.Join(fixturesDir, "links.yml"))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var fixtures LinkFixtures

	err = yaml.Unmarshal(data, &fixtures)
	if err != nil {
		return nil, err
	}

	return fixtures.Links, nil
}

// SeedLinks добавляет фикстуры ссылок в тестовую базу данных.
func SeedLinks(ctx context.Context, q *repository.Queries, links []LinkFixture) error {
	for _, l := range links {
		_, err := q.CreateLink(ctx, repository.CreateLinkParams{
			OriginalUrl: l.OriginalURL,
			ShortName:   l.ShortName,
			ShortUrl:    l.ShortURL,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// LoadLinkVisitFixtures загружает фикстуры посещений ссылок из testdata.
func LoadLinkVisitFixtures(t *testing.T) ([]LinkVisitFixture, error) {
	fixturesDir := fixturesPath(t)
	path := filepath.Clean(filepath.Join(fixturesDir, "link_visits.yml"))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var fixtures LinkVisitFixtures

	err = yaml.Unmarshal(data, &fixtures)
	if err != nil {
		return nil, err
	}

	return fixtures.LinkVisits, nil
}

// SeedLinkVisits добавляет фикстуры посещений ссылок в тестовую базу данных.
func SeedLinkVisits(ctx context.Context, q *repository.Queries, linkVisits []LinkVisitFixture) error {
	for _, visit := range linkVisits {
		link, err := q.LinkByShortName(ctx, visit.ShortName)
		if err != nil {
			return err
		}

		_, err = q.CreateLinkVisit(ctx, repository.CreateLinkVisitParams{
			LinkID:    link.ID,
			Ip:        visit.IP,
			UserAgent: visit.UserAgent,
			Referer:   visit.Referer,
			Status:    visit.Status,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func fixturesPath(t *testing.T) string {
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)

	dir := filepath.Dir(filename)
	return filepath.Clean(filepath.Join(dir, "..", "..", "testdata"))
}

func migrationsPath() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("failed to resolve migrations path")
	}

	dir := filepath.Dir(filename)
	return filepath.Clean(filepath.Join(dir, "..", "..", "db", "migrations"))
}
