package api

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type rangeBounds struct {
	start int
	end   int
}

type paginationParams struct {
	Limit  int32
	Offset int32
}

func getLimitAndOffsetFromQuery(rangeParam string) (paginationParams, rangeBounds, error) {
	pagination := paginationParams{}
	bounds := rangeBounds{}

	rangeParam = strings.TrimPrefix(rangeParam, "[")
	rangeParam = strings.TrimSuffix(rangeParam, "]")

	parts := strings.Split(rangeParam, ",")
	if len(parts) != 2 {
		return pagination, bounds, errors.New("incorrect range")
	}

	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return pagination, bounds, errors.New("incorrect range")
	}
	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return pagination, bounds, errors.New("incorrect range")
	}

	if start < 0 || end < start {
		return pagination, bounds, errors.New("incorrect range")
	}

	bounds = rangeBounds{start: start, end: end}

	return paginationParams{
		Limit:  int32(end - start),
		Offset: int32(start),
	}, bounds, nil
}

func buildContentRange(resource string, requestedRange *rangeBounds, count int, total int64) string {
	if requestedRange == nil {
		if count == 0 {
			return fmt.Sprintf("%s */0", resource)
		}
		return fmt.Sprintf("%s 0-%d/%d", resource, count-1, total)
	}

	if count == 0 {
		return fmt.Sprintf("%s */%d", resource, total)
	}

	end := requestedRange.start + count - 1
	return fmt.Sprintf("%s %d-%d/%d", resource, requestedRange.start, end, total)
}
