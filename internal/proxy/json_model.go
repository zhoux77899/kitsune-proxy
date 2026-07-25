package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	errInvalidJSON    = errors.New("invalid JSON object")
	errMissingModel   = errors.New("missing top-level model")
	errInvalidModel   = errors.New("top-level model must be a string")
	errDuplicateModel = errors.New("top-level model must appear exactly once")
)

type modelLocation struct {
	value string
	start int
	end   int
}

func locateTopLevelModel(data []byte) (modelLocation, error) {
	scanner := jsonScanner{data: data}
	scanner.skipWhitespace()
	if !scanner.consume('{') {
		return modelLocation{}, errInvalidJSON
	}

	var found *modelLocation
	scanner.skipWhitespace()
	if scanner.consume('}') {
		return modelLocation{}, errMissingModel
	}

	for {
		scanner.skipWhitespace()
		key, _, _, err := scanner.parseString()
		if err != nil {
			return modelLocation{}, errInvalidJSON
		}
		scanner.skipWhitespace()
		if !scanner.consume(':') {
			return modelLocation{}, errInvalidJSON
		}
		scanner.skipWhitespace()

		if key == "model" {
			if found != nil {
				return modelLocation{}, errDuplicateModel
			}
			if scanner.peek() != '"' {
				return modelLocation{}, errInvalidModel
			}
			value, start, end, err := scanner.parseString()
			if err != nil {
				return modelLocation{}, errInvalidJSON
			}
			location := modelLocation{value: value, start: start, end: end}
			found = &location
		} else if err := scanner.skipValue(1); err != nil {
			return modelLocation{}, errInvalidJSON
		}

		scanner.skipWhitespace()
		switch {
		case scanner.consume(','):
			continue
		case scanner.consume('}'):
			scanner.skipWhitespace()
			if scanner.index != len(scanner.data) {
				return modelLocation{}, errInvalidJSON
			}
			if found == nil {
				return modelLocation{}, errMissingModel
			}
			return *found, nil
		default:
			return modelLocation{}, errInvalidJSON
		}
	}
}

func replaceModel(data []byte, location modelLocation, target string) ([]byte, error) {
	encoded, err := json.Marshal(target)
	if err != nil {
		return nil, fmt.Errorf("encode target model: %w", err)
	}
	output := make([]byte, 0, len(data)-(location.end-location.start)+len(encoded))
	output = append(output, data[:location.start]...)
	output = append(output, encoded...)
	output = append(output, data[location.end:]...)
	return output, nil
}

type jsonScanner struct {
	data  []byte
	index int
}

func (s *jsonScanner) skipWhitespace() {
	for s.index < len(s.data) {
		switch s.data[s.index] {
		case ' ', '\t', '\r', '\n':
			s.index++
		default:
			return
		}
	}
}

func (s *jsonScanner) consume(value byte) bool {
	if s.index >= len(s.data) || s.data[s.index] != value {
		return false
	}
	s.index++
	return true
}

func (s *jsonScanner) peek() byte {
	if s.index >= len(s.data) {
		return 0
	}
	return s.data[s.index]
}

func (s *jsonScanner) parseString() (string, int, int, error) {
	start := s.index
	if !s.consume('"') {
		return "", 0, 0, errInvalidJSON
	}

	escaped := false
	for s.index < len(s.data) {
		current := s.data[s.index]
		s.index++
		if escaped {
			escaped = false
			continue
		}
		switch current {
		case '\\':
			escaped = true
		case '"':
			raw := s.data[start:s.index]
			var decoded string
			if err := json.Unmarshal(raw, &decoded); err != nil {
				return "", 0, 0, errInvalidJSON
			}
			return decoded, start, s.index, nil
		}
	}
	return "", 0, 0, errInvalidJSON
}

func (s *jsonScanner) skipValue(depth int) error {
	if depth > 1000 {
		return errInvalidJSON
	}
	s.skipWhitespace()
	switch s.peek() {
	case '"':
		_, _, _, err := s.parseString()
		return err
	case '{':
		return s.skipObject(depth + 1)
	case '[':
		return s.skipArray(depth + 1)
	case 't':
		return s.skipLiteral("true")
	case 'f':
		return s.skipLiteral("false")
	case 'n':
		return s.skipLiteral("null")
	default:
		return s.skipNumber()
	}
}

func (s *jsonScanner) skipObject(depth int) error {
	if !s.consume('{') {
		return errInvalidJSON
	}
	s.skipWhitespace()
	if s.consume('}') {
		return nil
	}
	for {
		s.skipWhitespace()
		if _, _, _, err := s.parseString(); err != nil {
			return err
		}
		s.skipWhitespace()
		if !s.consume(':') {
			return errInvalidJSON
		}
		if err := s.skipValue(depth); err != nil {
			return err
		}
		s.skipWhitespace()
		if s.consume('}') {
			return nil
		}
		if !s.consume(',') {
			return errInvalidJSON
		}
	}
}

func (s *jsonScanner) skipArray(depth int) error {
	if !s.consume('[') {
		return errInvalidJSON
	}
	s.skipWhitespace()
	if s.consume(']') {
		return nil
	}
	for {
		if err := s.skipValue(depth); err != nil {
			return err
		}
		s.skipWhitespace()
		if s.consume(']') {
			return nil
		}
		if !s.consume(',') {
			return errInvalidJSON
		}
	}
}

func (s *jsonScanner) skipLiteral(literal string) error {
	if !bytes.HasPrefix(s.data[s.index:], []byte(literal)) {
		return errInvalidJSON
	}
	s.index += len(literal)
	return nil
}

func (s *jsonScanner) skipNumber() error {
	start := s.index
	for s.index < len(s.data) {
		switch s.data[s.index] {
		case ' ', '\t', '\r', '\n', ',', '}', ']':
			goto done
		default:
			s.index++
		}
	}

done:
	if start == s.index || !json.Valid(s.data[start:s.index]) {
		return errInvalidJSON
	}
	return nil
}
