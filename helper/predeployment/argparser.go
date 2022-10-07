package predeployment

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

func ParseArguments(raws []string) ([]interface{}, error) {
	var (
		args   = make([]interface{}, len(raws))
		parser = &ArgParser{}

		err error
	)

	for idx, raw := range raws {
		if args[idx], err = parser.Parse(raw); err != nil {
			return nil, err
		}
	}

	return args, nil
}

type ArgParser struct {
	*strings.Reader
}

// Parse parse string and return one input args
func (p *ArgParser) Parse(input string) (interface{}, error) {
	p.Reader = strings.NewReader(input)

	return p.parseArgument()
}

// PeekRune reads one character from the current position without change of the position
func (p *ArgParser) PeekRune() (rune, error) {
	next, nextSize, err := p.ReadRune()
	if err != nil {
		return 0, err
	}

	// move backward consumed size
	if _, err := p.Seek(-1*int64(nextSize), 1); err != nil {
		return next, err
	}

	return next, nil
}

// consumeSpaces read all consecutive spaces from the current position
func (p *ArgParser) consumeSpaces() error {
	for {
		next, err := p.PeekRune()
		if err != nil {
			if errors.Is(io.EOF, err) {
				return nil
			}

			return err
		}

		if next != ' ' {
			return nil
		}

		// consume and continue loop
		if _, _, err := p.ReadRune(); err != nil {
			return err
		}
	}
}

// parseArgument defines the top level of parser grammars
func (p *ArgParser) parseArgument() (interface{}, error) {
	fst, err := p.PeekRune()
	if err != nil {
		return nil, err
	}

	switch fst {
	case ' ':
		if err := p.consumeSpaces(); err != nil {
			return nil, err
		}

		return p.parseArgument()
	case '"':
		return p.parseString()
	case '[':
		return p.parseArray()
	case ',', ']':
		return nil, fmt.Errorf("reached unexpected character: '%c'", fst)
	default:
		return p.parseLiteral()
	}
}

func (p *ArgParser) parseString() (string, error) {
	// consume the opening double quote
	opening, _, err := p.ReadRune()
	if err != nil {
		return "", err
	}

	var (
		chars     = []rune{} // parsed characters
		isEscaped = false    // is the next character escaped
	)

	for {
		next, _, err := p.ReadRune()
		if err != nil {
			return "", err
		}

		switch {
		case isEscaped:
			// if the previous character is escape symbol
			// then parse character directly
			chars = append(chars, next)

			isEscaped = false

		case next == '\\':
			isEscaped = true

		case next == '"' || next == '\'':
			if next != opening {
				return "", fmt.Errorf("bracket mismatch happened, opening=%c, closing=%c", opening, next)
			}

			return string(chars), nil

		default:
			chars = append(chars, next)
		}
	}
}

func (p *ArgParser) parseArray() ([]interface{}, error) {
	// consume the opening bracket
	if _, _, err := p.ReadRune(); err != nil {
		return nil, err
	}

	elems := []interface{}{}

	for {
		next, err := p.PeekRune()
		if err != nil {
			return nil, err
		}

		switch next {
		case ' ':
			if err := p.consumeSpaces(); err != nil {
				return nil, err
			}

		case ']':
			if _, _, err := p.ReadRune(); err != nil {
				return nil, err
			}

			return elems, nil

		case ',':
			if len(elems) == 0 {
				return nil, fmt.Errorf("invalid grammar, comma can't be as the first character in array")
			}

			if _, _, err := p.ReadRune(); err != nil {
				return nil, err
			}

			continue

		default:
			elem, err := p.parseArgument()
			if err != nil {
				return nil, err
			}

			elems = append(elems, elem)
		}
	}
}

// parseLiteral parses the single value except for string like number 123, 0xA0
func (p *ArgParser) parseLiteral() (interface{}, error) {
	chars := []rune{}

	for {
		next, err := p.PeekRune()
		if err != nil {
			if errors.Is(io.EOF, err) {
				return string(chars), nil
			}

			return "", err
		}

		switch next {
		case ',', ']':
			res := string(chars)

			if res == "true" {
				return true, nil
			} else if res == "false" {
				return false, nil
			}

			return res, nil

		case '[':
			return "", fmt.Errorf("invalid grammar, reached opening bracket in literal")

		default:
			next, _, err := p.ReadRune()
			if err != nil {
				return "", err
			}

			chars = append(chars, next)
		}
	}
}
