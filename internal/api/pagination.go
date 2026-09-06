package api

import (
	"net/http"
	"strconv"
)

type PaginationParams struct {
	Limit  int
	Offset int
}

func ParsePagination(r *http.Request) PaginationParams {
	limit := 100
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 1000 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}
	return PaginationParams{Limit: limit, Offset: offset}
}

type PaginationLinks struct {
	Self  string `json:"self"`
	Next  string `json:"next,omitempty"`
	Prev  string `json:"prev,omitempty"`
	First string `json:"first"`
	Last  string `json:"last,omitempty"`
}

func BuildLinks(r *http.Request, total, limit, offset int) PaginationLinks {
	base := r.URL.Path
	params := r.URL.Query()
	links := PaginationLinks{
		First: base + "?limit=" + strconv.Itoa(limit) + "&offset=0",
		Self:  base + "?limit=" + strconv.Itoa(limit) + "&offset=" + strconv.Itoa(offset),
	}
	if offset+limit < total {
		links.Next = base + "?limit=" + strconv.Itoa(limit) + "&offset=" + strconv.Itoa(offset+limit)
	}
	if offset > 0 {
		prev := offset - limit
		if prev < 0 {
			prev = 0
		}
		links.Prev = base + "?limit=" + strconv.Itoa(limit) + "&offset=" + strconv.Itoa(prev)
	}
	lastOffset := (total / limit) * limit
	if lastOffset >= total {
		lastOffset -= limit
	}
	if lastOffset < 0 {
		lastOffset = 0
	}
	links.Last = base + "?limit=" + strconv.Itoa(limit) + "&offset=" + strconv.Itoa(lastOffset)
	_ = params
	return links
}
