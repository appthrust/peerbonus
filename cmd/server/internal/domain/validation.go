package domain

import (
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/currency"
)

var valueKeyPattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
var monthKeyPattern = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)
var periodKeyPattern = regexp.MustCompile(`^[0-9]{4}-W(0[1-9]|[1-4][0-9]|5[0-3])$`)
var decimalPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func ValidateMessage(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) < 1 || utf8.RuneCountInString(s) > 500 {
		return "", &ValidationError{"message", "must contain 1 to 500 characters"}
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", &ValidationError{"message", "must not contain control characters"}
		}
	}
	return s, nil
}
func ValidatePoints(p, max int) error {
	if p < 1 || p > max {
		return &ValidationError{"points", "must be between 1 and the period's maximum"}
	}
	return nil
}
func ValidateValues(keys []string, program Program) error {
	if len(keys) > 3 {
		return &ValidationError{"values", "must contain at most 3 values"}
	}
	for i, key := range keys {
		if !valueKeyPattern.MatchString(key) {
			return &ValidationError{"values", "contains an invalid key"}
		}
		for _, previous := range keys[:i] {
			if previous == key {
				return &ValidationError{"values", "must not contain duplicate keys"}
			}
		}
		found := false
		for _, value := range program.Values {
			if value.Key == key {
				found = true
				break
			}
		}
		if !found {
			return ErrUnknownValue
		}
	}
	return nil
}
func ValidateProgramPatch(p ProgramPatch, current Program) error {
	if p.TimeZone != nil {
		if *p.TimeZone == "" || *p.TimeZone == "Local" {
			return &ValidationError{"timeZone", "must be an IANA time zone"}
		}
		if _, err := time.LoadLocation(*p.TimeZone); err != nil {
			return &ValidationError{"timeZone", "must be an IANA time zone"}
		}
	}
	allowance, max := current.WeeklyAllowance, current.MaxPointsPerPost
	if p.WeeklyAllowance != nil {
		allowance = *p.WeeklyAllowance
	}
	if p.MaxPointsPerPost != nil {
		max = *p.MaxPointsPerPost
	}
	if allowance < 0 || allowance > 10000 {
		return &ValidationError{"weeklyAllowance", "must be between 0 and 10000"}
	}
	if max < 1 || max > allowance {
		return &ValidationError{"maxPointsPerPost", "must be between 1 and weeklyAllowance"}
	}
	if p.PointUnitValue != nil && !decimalPattern.MatchString(*p.PointUnitValue) {
		return &ValidationError{"pointUnitValue", "must be a nonnegative decimal"}
	}
	if p.Currency != nil {
		if !currencyPattern.MatchString(*p.Currency) {
			return &ValidationError{"currency", "must be an uppercase ISO 4217 currency code"}
		}
		if _, err := currency.ParseISO(*p.Currency); err != nil {
			return &ValidationError{"currency", "must be an uppercase ISO 4217 currency code"}
		}
	}
	if p.Values != nil {
		values := *p.Values
		if len(values) > 20 {
			return &ValidationError{"values", "must contain at most 20 values"}
		}
		for i, value := range values {
			if !valueKeyPattern.MatchString(value.Key) || strings.TrimSpace(value.Label) == "" {
				return &ValidationError{"values", "each value needs a valid key and nonempty label"}
			}
			for _, previous := range values[:i] {
				if previous.Key == value.Key {
					return &ValidationError{"values", "must not contain duplicate keys"}
				}
			}
		}
	}
	return nil
}
func ValidateMonthKey(s string) error {
	if !monthKeyPattern.MatchString(s) {
		return ErrInvalidMonth
	}
	return nil
}
func ValidatePeriodKey(s string) error {
	if !periodKeyPattern.MatchString(s) {
		return &ValidationError{"period", "must be an ISO week key (YYYY-Www)"}
	}
	return nil
}
