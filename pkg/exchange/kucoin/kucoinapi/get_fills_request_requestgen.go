// Code generated by "requestgen -method GET -responseType .APIResponse -responseDataField Data -url /api/v1/fills -type GetFillsRequest -responseDataType .FillListPage"; DO NOT EDIT.

package kucoinapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

func (r *GetFillsRequest) OrderID(orderID string) *GetFillsRequest {
	r.orderID = &orderID
	return r
}

func (r *GetFillsRequest) TradeType(tradeType string) *GetFillsRequest {
	r.tradeType = &tradeType
	return r
}

func (r *GetFillsRequest) Symbol(symbol string) *GetFillsRequest {
	r.symbol = &symbol
	return r
}

func (r *GetFillsRequest) Side(side string) *GetFillsRequest {
	r.side = &side
	return r
}

func (r *GetFillsRequest) OrderType(orderType string) *GetFillsRequest {
	r.orderType = &orderType
	return r
}

func (r *GetFillsRequest) StartAt(startAt time.Time) *GetFillsRequest {
	r.startAt = &startAt
	return r
}

func (r *GetFillsRequest) EndAt(endAt time.Time) *GetFillsRequest {
	r.endAt = &endAt
	return r
}

// GetQueryParameters builds and checks the query parameters and returns url.Values
func (r *GetFillsRequest) GetQueryParameters() (url.Values, error) {
	var params = map[string]interface{}{}

	query := url.Values{}
	for k, v := range params {
		query.Add(k, fmt.Sprintf("%v", v))
	}

	return query, nil
}

// GetParameters builds and checks the parameters and return the result in a map object
func (r *GetFillsRequest) GetParameters() (map[string]interface{}, error) {
	var params = map[string]interface{}{}
	// check orderID field -> json key orderId
	if r.orderID != nil {
		orderID := *r.orderID

		// assign parameter of orderID
		params["orderId"] = orderID

	}
	// check tradeType field -> json key tradeType
	if r.tradeType != nil {
		tradeType := *r.tradeType

		// assign parameter of tradeType
		params["tradeType"] = tradeType

	}
	// check symbol field -> json key symbol
	if r.symbol != nil {
		symbol := *r.symbol

		// assign parameter of symbol
		params["symbol"] = symbol

	}
	// check side field -> json key side
	if r.side != nil {
		side := *r.side

		// TEMPLATE check-valid-values
		switch side {
		case "buy", "sell":
			params["side"] = side

		default:
			return params, fmt.Errorf("side value %v is invalid", side)

		}
		// END TEMPLATE check-valid-values

		// assign parameter of side
		params["side"] = side

	}
	// check orderType field -> json key type
	if r.orderType != nil {
		orderType := *r.orderType

		// TEMPLATE check-valid-values
		switch orderType {
		case "limit", "market", "limit_stop", "market_stop":
			params["type"] = orderType

		default:
			return params, fmt.Errorf("type value %v is invalid", orderType)

		}
		// END TEMPLATE check-valid-values

		// assign parameter of orderType
		params["type"] = orderType

	}
	// check startAt field -> json key startAt
	if r.startAt != nil {
		startAt := *r.startAt

		// assign parameter of startAt
		// convert time.Time to milliseconds time stamp
		params["startAt"] = strconv.FormatInt(startAt.UnixNano()/int64(time.Millisecond), 10)

	}
	// check endAt field -> json key endAt
	if r.endAt != nil {
		endAt := *r.endAt

		// assign parameter of endAt
		// convert time.Time to milliseconds time stamp
		params["endAt"] = strconv.FormatInt(endAt.UnixNano()/int64(time.Millisecond), 10)

	}

	return params, nil
}

// GetParametersQuery converts the parameters from GetParameters into the url.Values format
func (r *GetFillsRequest) GetParametersQuery() (url.Values, error) {
	query := url.Values{}

	params, err := r.GetParameters()
	if err != nil {
		return query, err
	}

	for k, v := range params {
		query.Add(k, fmt.Sprintf("%v", v))
	}

	return query, nil
}

// GetParametersJSON converts the parameters from GetParameters into the JSON format
func (r *GetFillsRequest) GetParametersJSON() ([]byte, error) {
	params, err := r.GetParameters()
	if err != nil {
		return nil, err
	}

	return json.Marshal(params)
}

// GetSlugParameters builds and checks the slug parameters and return the result in a map object
func (r *GetFillsRequest) GetSlugParameters() (map[string]interface{}, error) {
	var params = map[string]interface{}{}

	return params, nil
}

func (r *GetFillsRequest) applySlugsToUrl(url string, slugs map[string]string) string {
	for k, v := range slugs {
		needleRE := regexp.MustCompile(":" + k + "\\b")
		url = needleRE.ReplaceAllString(url, v)
	}

	return url
}

func (r *GetFillsRequest) GetSlugsMap() (map[string]string, error) {
	slugs := map[string]string{}
	params, err := r.GetSlugParameters()
	if err != nil {
		return slugs, nil
	}

	for k, v := range params {
		slugs[k] = fmt.Sprintf("%v", v)
	}

	return slugs, nil
}

func (r *GetFillsRequest) Do(ctx context.Context) (*FillListPage, error) {

	// empty params for GET operation
	var params interface{}
	query := url.Values{}

	apiURL := "/api/v1/fills"

	req, err := r.client.NewAuthenticatedRequest(ctx, "GET", apiURL, query, params)
	if err != nil {
		return nil, err
	}

	response, err := r.client.SendRequest(req)
	if err != nil {
		return nil, err
	}

	var apiResponse APIResponse
	if err := response.DecodeJSON(&apiResponse); err != nil {
		return nil, err
	}
	var data FillListPage
	if err := json.Unmarshal(apiResponse.Data, &data); err != nil {
		return nil, err
	}
	return &data, nil
}