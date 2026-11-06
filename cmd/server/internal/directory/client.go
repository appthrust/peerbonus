// Package directory reads authoritative people through the built-in app link.
package directory

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	contract "github.com/appthrust/appserver-go/appserver/contract/directory"
	"github.com/appthrust/appserver-go/appserver/link"
)

type Client struct{ links *link.Client }

func New(links *link.Client) *Client { return &Client{links: links} }

func (c *Client) Me(ctx context.Context, bearer string) (contract.Person, error) {
	if c.links == nil {
		return contract.Person{}, link.ErrAbsent
	}
	var response contract.MeResponse
	err := c.links.Do(ctx, bearer, contract.Me, link.Params{}, nil, &response)
	if err == nil && response.Person.ID == "" {
		err = link.ErrUnavailable
	}
	return response.Person, err
}

func (c *Client) Profile(ctx context.Context, bearer, personID string) (contract.PersonProfile, error) {
	if c.links == nil {
		return contract.PersonProfile{}, link.ErrAbsent
	}
	var response contract.PersonProfile
	err := c.links.Do(ctx, bearer, contract.GetPerson, link.Params{Path: map[string]string{"id": personID}}, nil, &response)
	if err == nil && response.Person.ID != personID {
		err = link.ErrUnavailable
	}
	return response, err
}

func (c *Client) Avatar(ctx context.Context, bearer, personID string) (body []byte, contentType string, err error) {
	if c.links == nil {
		return nil, "", link.ErrAbsent
	}
	op := link.Operation{Definition: "directory", ID: "getPersonAvatar", Method: http.MethodGet, Base: "/api/directory/v1", Path: "/people/{id}/avatar"}
	return c.links.Fetch(ctx, bearer, op, link.Params{Path: map[string]string{"id": personID}})
}

// PeopleByIDs respects Directory's 100-ID request limit. A failed batch returns
// the preceding successful batches too, so close can refresh those profiles.
func (c *Client) PeopleByIDs(ctx context.Context, bearer string, ids []string) ([]contract.Person, error) {
	people := make([]contract.Person, 0, len(ids))
	if len(ids) == 0 {
		return people, nil
	}
	if c.links == nil {
		return nil, link.ErrAbsent
	}
	for start := 0; start < len(ids); start += 100 {
		end := min(start+100, len(ids))
		var response contract.PersonList
		err := c.links.Do(ctx, bearer, contract.ListPeople, link.Params{Query: url.Values{"ids": {strings.Join(ids[start:end], ",")}, "limit": {"100"}}}, nil, &response)
		if err != nil {
			return people, err
		}
		people = append(people, response.Items...)
	}
	return people, nil
}

func (c *Client) Search(ctx context.Context, bearer, q string, limit int) ([]contract.Person, error) {
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	if q != "" {
		query.Set("q", q)
	}
	return c.listPeople(ctx, bearer, query)
}

func (c *Client) PeopleInGroup(ctx context.Context, bearer, groupID string, limit int) ([]contract.Person, error) {
	return c.listPeople(ctx, bearer, url.Values{"groupId": {groupID}, "limit": {strconv.Itoa(limit)}})
}

// Groups uses Directory's maximum list bound, like the people roster.
func (c *Client) Groups(ctx context.Context, bearer string) ([]contract.Group, error) {
	if c.links == nil {
		return nil, link.ErrAbsent
	}
	var response contract.GroupList
	err := c.links.Do(ctx, bearer, contract.ListGroups, link.Params{Query: url.Values{"limit": {"500"}}}, nil, &response)
	return response.Items, err
}

func (c *Client) ListAll(ctx context.Context, bearer string, limit int) ([]contract.Person, error) {
	return c.Search(ctx, bearer, "", limit)
}

func (c *Client) listPeople(ctx context.Context, bearer string, query url.Values) ([]contract.Person, error) {
	if c.links == nil {
		return nil, link.ErrAbsent
	}
	var response contract.PersonList
	err := c.links.Do(ctx, bearer, contract.ListPeople, link.Params{Query: query}, nil, &response)
	return response.Items, err
}
