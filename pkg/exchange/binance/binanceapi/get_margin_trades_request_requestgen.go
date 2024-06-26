// Code generated by "requestgen -method GET -url /sapi/v1/margin/myTrades -type GetMarginTradesRequest -responseType []Trade"; DO NOT EDIT.

package binanceapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/adshao/go-binance/v2"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"time"
)

func (g *GetMarginTradesRequest) IsIsolated(isIsolated bool) *GetMarginTradesRequest {
	g.isIsolated = isIsolated
	return g
}

func (g *GetMarginTradesRequest) Symbol(symbol string) *GetMarginTradesRequest {
	g.symbol = symbol
	return g
}

func (g *GetMarginTradesRequest) OrderID(orderID uint64) *GetMarginTradesRequest {
	g.orderID = &orderID
	return g
}

func (g *GetMarginTradesRequest) StartTime(startTime time.Time) *GetMarginTradesRequest {
	g.startTime = &startTime
	return g
}

func (g *GetMarginTradesRequest) EndTime(endTime time.Time) *GetMarginTradesRequest {
	g.endTime = &endTime
	return g
}

func (g *GetMarginTradesRequest) FromID(fromID uint64) *GetMarginTradesRequest {
	g.fromID = &fromID
	return g
}

func (g *GetMarginTradesRequest) Limit(limit uint64) *GetMarginTradesRequest {
	g.limit = &limit
	return g
}

// GetQueryParameters builds and checks the query parameters and returns url.Values
func (g *GetMarginTradesRequest) GetQueryParameters() (url.Values, error) {
	var params = map[string]interface{}{}

	query := url.Values{}
	for _k, _v := range params {
		query.Add(_k, fmt.Sprintf("%v", _v))
	}

	return query, nil
}

// GetParameters builds and checks the parameters and return the result in a map object
func (g *GetMarginTradesRequest) GetParameters() (map[string]interface{}, error) {
	var params = map[string]interface{}{}
	// check isIsolated field -> json key isIsolated
	isIsolated := g.isIsolated

	// assign parameter of isIsolated
	params["isIsolated"] = isIsolated
	// check symbol field -> json key symbol
	symbol := g.symbol

	// assign parameter of symbol
	params["symbol"] = symbol
	// check orderID field -> json key orderId
	if g.orderID != nil {
		orderID := *g.orderID

		// assign parameter of orderID
		params["orderId"] = orderID
	} else {
	}
	// check startTime field -> json key startTime
	if g.startTime != nil {
		startTime := *g.startTime

		// assign parameter of startTime
		// convert time.Time to milliseconds time stamp
		params["startTime"] = strconv.FormatInt(startTime.UnixNano()/int64(time.Millisecond), 10)
	} else {
	}
	// check endTime field -> json key endTime
	if g.endTime != nil {
		endTime := *g.endTime

		// assign parameter of endTime
		// convert time.Time to milliseconds time stamp
		params["endTime"] = strconv.FormatInt(endTime.UnixNano()/int64(time.Millisecond), 10)
	} else {
	}
	// check fromID field -> json key fromId
	if g.fromID != nil {
		fromID := *g.fromID

		// assign parameter of fromID
		params["fromId"] = fromID
	} else {
	}
	// check limit field -> json key limit
	if g.limit != nil {
		limit := *g.limit

		// assign parameter of limit
		params["limit"] = limit
	} else {
	}

	return params, nil
}

// GetParametersQuery converts the parameters from GetParameters into the url.Values format
func (g *GetMarginTradesRequest) GetParametersQuery() (url.Values, error) {
	query := url.Values{}

	params, err := g.GetParameters()
	if err != nil {
		return query, err
	}

	for _k, _v := range params {
		if g.isVarSlice(_v) {
			g.iterateSlice(_v, func(it interface{}) {
				query.Add(_k+"[]", fmt.Sprintf("%v", it))
			})
		} else {
			query.Add(_k, fmt.Sprintf("%v", _v))
		}
	}

	return query, nil
}

// GetParametersJSON converts the parameters from GetParameters into the JSON format
func (g *GetMarginTradesRequest) GetParametersJSON() ([]byte, error) {
	params, err := g.GetParameters()
	if err != nil {
		return nil, err
	}

	return json.Marshal(params)
}

// GetSlugParameters builds and checks the slug parameters and return the result in a map object
func (g *GetMarginTradesRequest) GetSlugParameters() (map[string]interface{}, error) {
	var params = map[string]interface{}{}

	return params, nil
}

func (g *GetMarginTradesRequest) applySlugsToUrl(url string, slugs map[string]string) string {
	for _k, _v := range slugs {
		needleRE := regexp.MustCompile(":" + _k + "\\b")
		url = needleRE.ReplaceAllString(url, _v)
	}

	return url
}

func (g *GetMarginTradesRequest) iterateSlice(slice interface{}, _f func(it interface{})) {
	sliceValue := reflect.ValueOf(slice)
	for _i := 0; _i < sliceValue.Len(); _i++ {
		it := sliceValue.Index(_i).Interface()
		_f(it)
	}
}

func (g *GetMarginTradesRequest) isVarSlice(_v interface{}) bool {
	rt := reflect.TypeOf(_v)
	switch rt.Kind() {
	case reflect.Slice:
		return true
	}
	return false
}

func (g *GetMarginTradesRequest) GetSlugsMap() (map[string]string, error) {
	slugs := map[string]string{}
	params, err := g.GetSlugParameters()
	if err != nil {
		return slugs, nil
	}

	for _k, _v := range params {
		slugs[_k] = fmt.Sprintf("%v", _v)
	}

	return slugs, nil
}

// GetPath returns the request path of the API
func (g *GetMarginTradesRequest) GetPath() string {
	return "/sapi/v1/margin/myTrades"
}

// Do generates the request object and send the request object to the API endpoint
func (g *GetMarginTradesRequest) Do(ctx context.Context) ([]binance.TradeV3, error) {

	// empty params for GET operation
	var params interface{}
	query, err := g.GetParametersQuery()
	if err != nil {
		return nil, err
	}

	var apiURL string

	apiURL = g.GetPath()

	req, err := g.client.NewAuthenticatedRequest(ctx, "GET", apiURL, query, params)
	if err != nil {
		return nil, err
	}

	response, err := g.client.SendRequest(req)
	if err != nil {
		return nil, err
	}

	var apiResponse []binance.TradeV3

	type responseUnmarshaler interface {
		Unmarshal(data []byte) error
	}

	if unmarshaler, ok := interface{}(&apiResponse).(responseUnmarshaler); ok {
		if err := unmarshaler.Unmarshal(response.Body); err != nil {
			return nil, err
		}
	} else {
		// The line below checks the content type, however, some API server might not send the correct content type header,
		// Hence, this is commented for backward compatibility
		// response.IsJSON()
		if err := response.DecodeJSON(&apiResponse); err != nil {
			return nil, err
		}
	}

	type responseValidator interface {
		Validate() error
	}

	if validator, ok := interface{}(&apiResponse).(responseValidator); ok {
		if err := validator.Validate(); err != nil {
			return nil, err
		}
	}
	return apiResponse, nil
}
