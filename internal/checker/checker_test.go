package checker

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pyromeowww/uptime-checker/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			got := NormalizeURL(tt.input)
			assert.Equal(t, tt.expected, got)
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

	// require останавливает тест при ошибке
	require.NoError(t, err)
	require.Len(t, res, 1)

	// assert проверяет равенство и фиксирует ошибку
	assert.Equal(t, domain.StatusUp, res[0].Status)
	assert.Equal(t, http.StatusOK, res[0].StatusCode)
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

		require.NoError(t, err)
		require.Len(t, res, 1)

		assert.Equal(t, domain.StatusDown, res[0].Status)
		assert.Equal(t, http.StatusNotFound, res[0].StatusCode)
	})

	// Сервер вообще недоступен
	t.Run("Network error / Unreachable host", func(t *testing.T) {
		res, err := chk.Check("http://invalid-domain-123456789-test.org")

		require.NoError(t, err)
		require.Len(t, res, 1)

		assert.Equal(t, domain.StatusDown, res[0].Status)
		assert.NotEmpty(t, res[0].ErrMsg)
	})
}

func TestCheck_Parallel(t *testing.T) {
	// Поднимаем два фейковых сервера
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server2.Close()

	chk := NewChecker()
	// Передаем сразу два URL
	results, err := chk.Check(server1.URL, server2.URL)

	require.NoError(t, err)
	require.Len(t, results, 2)

	// Собираем полученные кодовые статусы
	codes := []int{results[0].StatusCode, results[1].StatusCode}

	// Проверяем, что в результатах присутствуют и 200, и 404
	assert.Contains(t, codes, http.StatusOK)
	assert.Contains(t, codes, http.StatusNotFound)
}
