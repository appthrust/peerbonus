package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPeriodAndMonthBoundaries(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	for _, tc := range []struct{ at, week, month, start, end string }{
		{"2026-10-18T23:59:59+09:00", "2026-W42", "2026-10", "2026-10-12T00:00:00+09:00", "2026-10-19T00:00:00+09:00"},
		{"2026-10-19T00:00:00+09:00", "2026-W43", "2026-10", "2026-10-19T00:00:00+09:00", "2026-10-26T00:00:00+09:00"},
		{"2027-01-01T00:00:00+09:00", "2026-W53", "2027-01", "2026-12-28T00:00:00+09:00", "2027-01-04T00:00:00+09:00"},
		{"2027-01-04T00:00:00+09:00", "2027-W01", "2027-01", "2027-01-04T00:00:00+09:00", "2027-01-11T00:00:00+09:00"},
		{"2018-12-31T00:00:00+09:00", "2019-W01", "2018-12", "2018-12-31T00:00:00+09:00", "2019-01-07T00:00:00+09:00"},
		{"2026-10-31T15:00:00Z", "2026-W44", "2026-11", "2026-10-26T00:00:00+09:00", "2026-11-02T00:00:00+09:00"},
	} {
		t.Run(tc.at, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, tc.at)
			require.NoError(t, err)
			key, start, end := PeriodFor(at, loc)
			require.Equal(t, tc.week, key)
			require.Equal(t, tc.start, start.Format(time.RFC3339))
			require.Equal(t, tc.end, end.Format(time.RFC3339))
			require.Equal(t, tc.month, MonthKeyFor(at, loc))
		})
	}
}
func TestPeriodUsesCalendarMidnightAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	for _, tc := range []struct {
		at    string
		hours int
	}{{"2026-03-08T12:00:00-04:00", 167}, {"2026-11-01T12:00:00-05:00", 169}} {
		at, err := time.Parse(time.RFC3339, tc.at)
		require.NoError(t, err)
		_, start, end := PeriodFor(at, loc)
		require.Equal(t, 0, start.Hour())
		require.Equal(t, 0, end.Hour())
		require.Equal(t, time.Duration(tc.hours)*time.Hour, end.Sub(start))
	}
}
func TestMonthEnded(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	end := time.Date(2026, 11, 1, 0, 0, 0, 0, loc)
	ended, err := MonthEnded("2026-10", end.Add(-time.Nanosecond), loc)
	require.NoError(t, err)
	require.False(t, ended)
	ended, err = MonthEnded("2026-10", end, loc)
	require.NoError(t, err)
	require.True(t, ended)
	ended, err = MonthEnded("2026-13", end, loc)
	require.ErrorIs(t, err, ErrInvalidMonth)
	require.False(t, ended)
	ended, err = MonthEnded("2024-02", time.Date(2024, 3, 1, 0, 0, 0, 0, loc), loc)
	require.NoError(t, err)
	require.True(t, ended)
}
func TestMessageValidation(t *testing.T) {
	for _, input := range []string{"ありがとう", "line\nnext\ttab", strings.Repeat("あ", 500)} {
		actual, err := ValidateMessage("  " + input + "  ")
		require.NoError(t, err)
		require.Equal(t, input, actual)
	}
	for _, input := range []string{"", " \n\t", strings.Repeat("あ", 501), "a\x00b", "a\rb", "a\x7fb", string([]byte{0xff})} {
		_, err := ValidateMessage(input)
		var validation *ValidationError
		require.ErrorAs(t, err, &validation)
		require.Equal(t, "message", validation.Field)
	}
}
func TestPointAndValueValidation(t *testing.T) {
	require.NoError(t, ValidatePoints(1, 120))
	require.NoError(t, ValidatePoints(120, 120))
	require.Error(t, ValidatePoints(0, 120))
	require.Error(t, ValidatePoints(121, 120))
	program := Program{Values: []Value{{Key: "care"}, {Key: "learn"}, {Key: "challenge"}}}
	require.NoError(t, ValidateValues(nil, program))
	require.NoError(t, ValidateValues([]string{"care", "learn", "challenge"}, program))
	require.ErrorIs(t, ValidateValues([]string{"unknown"}, program), ErrUnknownValue)
	for _, keys := range [][]string{{"care", "care"}, {"Bad"}, {""}, {strings.Repeat("a", 33)}, {"care", "learn", "challenge", "fourth"}} {
		var validation *ValidationError
		require.ErrorAs(t, ValidateValues(keys, program), &validation)
	}
}
func TestProgramValidation(t *testing.T) {
	current := Program{TimeZone: "Asia/Tokyo", WeeklyAllowance: 400, MaxPointsPerPost: 120, PointUnitValue: "1", Currency: "JPY"}
	for _, patch := range []ProgramPatch{
		{}, {TimeZone: new("UTC")}, {WeeklyAllowance: new(10000), MaxPointsPerPost: new(10000)},
		{WeeklyAllowance: new(1), MaxPointsPerPost: new(1)}, {PointUnitValue: new("0.0001"), Currency: new("USD")},
		{Values: new([]Value{{Key: "challenge", Label: "挑戦"}})},
	} {
		require.NoError(t, ValidateProgramPatch(patch, current))
	}
	for _, patch := range []ProgramPatch{
		{TimeZone: new("Invalid/Zone")}, {TimeZone: new("")}, {TimeZone: new("Local")},
		{WeeklyAllowance: new(-1)}, {WeeklyAllowance: new(10001)}, {WeeklyAllowance: new(0)}, {WeeklyAllowance: new(100)},
		{MaxPointsPerPost: new(0)}, {MaxPointsPerPost: new(401)}, {PointUnitValue: new("-0.1")},
		{PointUnitValue: new("NaN")}, {PointUnitValue: new("Infinity")}, {PointUnitValue: new("1/2")}, {PointUnitValue: new("1e3")},
		{Currency: new("jpy")}, {Currency: new("US")}, {Currency: new("ZZZ")},
		{Values: new(make([]Value, 21))}, {Values: new([]Value{{Key: "BAD", Label: "Invalid"}})},
		{Values: new([]Value{{Key: "good", Label: " "}})}, {Values: new([]Value{{Key: "care", Label: "Care"}, {Key: "care", Label: "Again"}})},
	} {
		var validation *ValidationError
		require.ErrorAs(t, ValidateProgramPatch(patch, current), &validation)
	}
}
func TestKeyValidationAndErrorTypes(t *testing.T) {
	require.NoError(t, ValidateMonthKey("2026-01"))
	for _, key := range []string{"2026-00", "2026-13", "2026-1", "26-01", "2026-01-01"} {
		require.ErrorIs(t, ValidateMonthKey(key), ErrInvalidMonth)
	}
	for _, key := range []string{"2026-W01", "2026-W53"} {
		require.NoError(t, ValidatePeriodKey(key))
	}
	for _, key := range []string{"2026-W00", "2026-W54", "2026-W1", "2026-w01"} {
		require.Error(t, ValidatePeriodKey(key))
	}
	var insufficient *InsufficientPointsError
	require.True(t, errors.As(&InsufficientPointsError{Remaining: 1, Required: 2}, &insufficient))
	require.Equal(t, "insufficient_points: remaining 1, required 2", insufficient.Error())
}
