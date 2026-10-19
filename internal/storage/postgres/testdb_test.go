package postgres_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// checkTestDatabaseURL - защита от запуска интеграционных тестов на рабочей базе.
// Тесты применяют миграции и пишут в таблицы, поэтому база разрешена, только если:
//   - в имени базы есть "test" (например, petshop_test);
//   - это не та же база, что в DATABASE_URL (тот же хост, порт и имя).
func checkTestDatabaseURL(testURL, mainURL string) error {
	testCfg, err := pgx.ParseConfig(testURL)
	if err != nil {
		return fmt.Errorf("cannot parse TEST_DATABASE_URL: %w", err)
	}

	if !strings.Contains(strings.ToLower(testCfg.Database), "test") {
		return fmt.Errorf("database %q in TEST_DATABASE_URL does not look like a test database: "+
			"its name must contain \"test\", for example petshop_test", testCfg.Database)
	}

	if mainURL == "" {
		return nil
	}
	mainCfg, err := pgx.ParseConfig(mainURL)
	if err != nil {
		// DATABASE_URL нужен только для сравнения; если он битый, совпасть с тестовой базой он не может
		return nil
	}
	if strings.EqualFold(testCfg.Host, mainCfg.Host) &&
		testCfg.Port == mainCfg.Port &&
		testCfg.Database == mainCfg.Database {
		return fmt.Errorf("TEST_DATABASE_URL points to the same database as DATABASE_URL (%s:%d/%s)",
			testCfg.Host, testCfg.Port, testCfg.Database)
	}

	return nil
}

func TestCheckTestDatabaseURL(t *testing.T) {
	const prodURL = "postgres://user:pass@localhost:5432/petshop_test?sslmode=disable"

	tests := []struct {
		name    string
		testURL string
		mainURL string
		wantErr bool
	}{
		{"separate test database", "postgres://user:pass@localhost:5432/petshop_test", "postgres://user:pass@localhost:5432/petshop", false},
		{"test database without DATABASE_URL", "postgres://user:pass@localhost:5433/test", "", false},
		{"name without test", "postgres://user:pass@localhost:5432/petshop", "", true},
		{"default postgres database", "postgres://user:pass@localhost:5432/postgres", "", true},
		{"same database as DATABASE_URL", prodURL, prodURL, true},
		{"same database, different credentials and host case", "postgres://other:secret@LOCALHOST:5432/petshop_test", prodURL, true},
		{"same name on another port", "postgres://user:pass@localhost:5433/petshop_test", prodURL, false},
		{"unparsable url", "postgres://user:pass@localhost:notaport/petshop_test", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkTestDatabaseURL(tt.testURL, tt.mainURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkTestDatabaseURL(%q, %q) error = %v, wantErr %v", tt.testURL, tt.mainURL, err, tt.wantErr)
			}
		})
	}
}
