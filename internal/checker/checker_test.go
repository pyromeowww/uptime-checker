package checker

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pyromeowww/uptime-checker/internal/domain"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string // Сырой пример URL
		expected string // Идеальный ULR
	}{
		{name: "без протокола", input: "github.com", expected: "http://github.com"},
		{name: "с протоколом https", input: "https://github.com", expected: "https://github.com"},
		{name: "в верхнем регистре", input: "HTTPS://GITHUB.COM", expected: "https://github.com"},
		{name: "с пробелом", input: "     https://github.com    ", expected: "https://github.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeURL(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeURL(%q) = %q; ожидалось: %q", tt.input, got, tt.expected)
			}
		})
	}
}

// Тест для успешного сценария
func TestCheck_Success(t *testing.T) {
	// Поднимаем фейковый сервер, который всегда отвечает 200 OK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	chk := NewChecker()
	res, err := chk.Check(server.URL)

	if err != nil {
		t.Fatalf("не ожидали ошибку Go, но получили: %v", err)
	}
	if res.Status != domain.StatusUp {
		t.Errorf("ожидали статус %s, но получили %s", domain.StatusUp, res.Status)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("ожидали HTTP-код %d, получили %d", http.StatusOK, res.StatusCode)
	}
}

// Тест для сценариев, когда сервер отвечает ошибкой или недоступен
func TestCheck_Failure(t *testing.T) {
	// Сервер доступен, но отдаёт HTTP-ошибку
	chk := NewChecker()
	t.Run("server return 404", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		res, err := chk.Check(server.URL)
		if err != nil {
			t.Fatalf("не ожидали ошибку Go, но получили: %v", err)
		}
		if res.Status != domain.StatusDown {
			t.Errorf("ожидали статус %s, но получили %s", domain.StatusDown, res.Status)
		}
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("ожидали HTTP-код %d, получили %d", http.StatusNotFound, res.StatusCode)
		}
	})

	// Сервер вообще недоступен
	t.Run("Network error / Unreachable host", func(t *testing.T) {
		res, err := chk.Check("http://invalid-domain-123456789-test.org")
		if err != nil {
			t.Fatalf("не ожидали ошибку Go, но получили: %v", err)
		}
		if res.Status != domain.StatusDown {
			t.Errorf("ожидали статус %s, но получили %s", domain.StatusDown, res.Status)
		}
		if res.ErrMsg == "" {
			t.Error("ожидали, что ErrMsg будет заполнено текстом ошибки")
		}
	})
}
