package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetLimitAndOffsetFromQuery проверяет разбор корректных range-параметров.
func TestGetLimitAndOffsetFromQuery(t *testing.T) {
	tests := []struct {
		name               string
		rangeParam         string
		expectedPagination paginationParams
		expectedBounds     rangeBounds
	}{
		{
			name:       "parses range from zero",
			rangeParam: "[0,10]",
			expectedPagination: paginationParams{
				Limit:  10,
				Offset: 0,
			},
			expectedBounds: rangeBounds{
				start: 0,
				end:   10,
			},
		},
		{
			name:       "parses range with offset",
			rangeParam: "[5,15]",
			expectedPagination: paginationParams{
				Limit:  10,
				Offset: 5,
			},
			expectedBounds: rangeBounds{
				start: 5,
				end:   15,
			},
		},
		{
			name:       "trims whitespace around bounds",
			rangeParam: "[ 2, 7 ]",
			expectedPagination: paginationParams{
				Limit:  5,
				Offset: 2,
			},
			expectedBounds: rangeBounds{
				start: 2,
				end:   7,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pagination, bounds, err := getLimitAndOffsetFromQuery(tt.rangeParam)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedPagination, pagination)
			assert.Equal(t, tt.expectedBounds, bounds)
		})
	}
}

// TestGetLimitAndOffsetFromQueryReturnsErrorForIncorrectRange проверяет ошибки для некорректных range-параметров.
func TestGetLimitAndOffsetFromQueryReturnsErrorForIncorrectRange(t *testing.T) {
	tests := []struct {
		name       string
		rangeParam string
	}{
		{
			name:       "empty range",
			rangeParam: "",
		},
		{
			name:       "missing end bound",
			rangeParam: "[1]",
		},
		{
			name:       "extra bound",
			rangeParam: "[1,2,3]",
		},
		{
			name:       "non numeric start bound",
			rangeParam: "[first,2]",
		},
		{
			name:       "non numeric end bound",
			rangeParam: "[1,last]",
		},
		{
			name:       "negative start bound",
			rangeParam: "[-1,2]",
		},
		{
			name:       "end before start",
			rangeParam: "[3,2]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pagination, bounds, err := getLimitAndOffsetFromQuery(tt.rangeParam)

			require.Error(t, err)
			assert.EqualError(t, err, "incorrect range")
			assert.Equal(t, paginationParams{}, pagination)
			assert.Equal(t, rangeBounds{}, bounds)
		})
	}
}

// TestBuildContentRange проверяет форматирование заголовка Content-Range.
func TestBuildContentRange(t *testing.T) {
	tests := []struct {
		name           string
		resource       string
		requestedRange *rangeBounds
		count          int
		total          int64
		expected       string
	}{
		{
			name:     "builds range without requested bounds",
			resource: "links",
			count:    3,
			total:    10,
			expected: "links 0-2/10",
		},
		{
			name:     "builds empty range without requested bounds",
			resource: "links",
			count:    0,
			total:    0,
			expected: "links */0",
		},
		{
			name:           "builds range from requested start",
			resource:       "link_visits",
			requestedRange: &rangeBounds{start: 5, end: 15},
			count:          2,
			total:          12,
			expected:       "link_visits 5-6/12",
		},
		{
			name:           "builds empty range with requested bounds",
			resource:       "link_visits",
			requestedRange: &rangeBounds{start: 5, end: 15},
			count:          0,
			total:          12,
			expected:       "link_visits */12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, buildContentRange(tt.resource, tt.requestedRange, tt.count, tt.total))
		})
	}
}
