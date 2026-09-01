// Package fuzzy ranks short resource names by boundaries and consecutive runs rather than incidental matches.
package fuzzy

import (
	"sort"
	"strings"
	"unicode"
)

// These values preserve fzf's ranking contract; changing one independently can invert resource ordering.
const (
	scoreMatch        = 16
	scoreGapStart     = -3
	scoreGapExtension = -1

	bonusBoundary = scoreMatch / 2

	bonusNonWord = scoreMatch / 2

	bonusCamel123 = bonusBoundary + scoreGapExtension

	// Cancelling the gap penalty guarantees an unbroken run beats the same scattered characters.
	bonusConsecutive = -(scoreGapStart + scoreGapExtension)

	bonusFirstCharMultiplier = 2
)

// delimiters treats AWS identifier punctuation as word boundaries.
const delimiters = ":/-_.,;| "

type charClass int

// charWhite begins fzf's ordering contract where word classes rank above charNonWord.
const (
	charWhite charClass = iota
	charNonWord
	charDelimiter
	charLower
	charUpper
	charNumber
)

func classOf(r rune) charClass {
	switch {
	case unicode.IsUpper(r):
		return charUpper
	case unicode.IsLower(r):
		return charLower
	case unicode.IsDigit(r):
		return charNumber
	case unicode.IsSpace(r):
		return charWhite
	case strings.ContainsRune(delimiters, r):
		return charDelimiter
	case unicode.IsLetter(r):
		// Caseless scripts must still score as word characters.
		return charLower
	default:
		return charNonWord
	}
}

func bonusFor(prev, cur charClass) int {
	if cur > charNonWord {
		switch prev {
		case charWhite, charDelimiter, charNonWord:
			return bonusBoundary
		}
	}

	if prev == charLower && cur == charUpper || prev != charNumber && cur == charNumber {
		return bonusCamel123
	}

	switch cur {
	case charNonWord, charDelimiter, charWhite:
		return bonusNonWord
	}

	return 0
}

type Result struct {
	Text      string
	Score     int
	Positions []int
}

// Match returns rune indexes for terminal highlighting; an empty pattern matches everything with score zero.
func Match(pattern, text string) (score int, positions []int, ok bool) {
	if pattern == "" {
		return 0, nil, true
	}

	pat := []rune(strings.ToLower(pattern))
	runes := []rune(text)
	lower := make([]rune, len(runes))
	for i, r := range runes {
		lower[i] = unicode.ToLower(r)
	}

	// Rerunning V1 from each start avoids leftmost-window bias; port V2 if candidates grow beyond short resource names.
	best, bestPositions := 0, []int(nil)
	for from := range lower {
		if lower[from] != pat[0] {
			continue
		}
		start, end, ok := window(pat, lower, from)
		if !ok {
			// No completion from here means none from any later start either.
			break
		}
		score, positions := scoreWindow(pat, runes, lower, start, end)
		if bestPositions == nil || score > best {
			best, bestPositions = score, positions
		}
	}
	if bestPositions == nil {
		return 0, nil, false
	}

	return best, bestPositions, true
}

func window(pat, lower []rune, from int) (start, end int, ok bool) {
	pidx := 0
	for i := from; i < len(lower); i++ {
		if lower[i] != pat[pidx] {
			continue
		}
		pidx++
		if pidx == len(pat) {
			end = i + 1
			ok = true
			break
		}
	}
	if !ok {
		return 0, 0, false
	}

	pidx = len(pat) - 1
	start = from
	for i := end - 1; i >= from; i-- {
		if lower[i] != pat[pidx] {
			continue
		}
		pidx--
		if pidx < 0 {
			start = i
			break
		}
	}

	return start, end, true
}

func scoreWindow(pat, runes, lower []rune, start, end int) (int, []int) {
	prev := charWhite
	if start > 0 {
		prev = classOf(runes[start-1])
	}

	positions := make([]int, 0, len(pat))
	score, pidx, consecutive, firstBonus := 0, 0, 0, 0
	inGap := false

	for i := start; i < end; i++ {
		cur := classOf(runes[i])
		bonus := bonusFor(prev, cur)
		prev = cur

		if pidx < len(pat) && lower[i] == pat[pidx] {
			positions = append(positions, i)
			score += scoreMatch

			if consecutive == 0 {
				firstBonus = bonus
			} else {
				// A run keeps the best bonus it started with, so a boundary hit carries through the characters that follow it.
				if bonus >= bonusBoundary && bonus > firstBonus {
					firstBonus = bonus
				}
				bonus = max(bonus, firstBonus, bonusConsecutive)
			}

			if pidx == 0 {
				score += bonus * bonusFirstCharMultiplier
			} else {
				score += bonus
			}

			inGap = false
			consecutive++
			pidx++
			continue
		}

		if inGap {
			score += scoreGapExtension
		} else {
			score += scoreGapStart
		}
		inGap = true
		consecutive = 0
		firstBonus = 0
	}

	return score, positions
}

// Rank preserves stable ties so equal candidates cannot shuffle between keystrokes.
func Rank(pattern string, candidates []string) []Result {
	results := make([]Result, 0, len(candidates))
	for _, candidate := range candidates {
		if score, positions, ok := Match(pattern, candidate); ok {
			results = append(results, Result{Text: candidate, Score: score, Positions: positions})
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if len(results[i].Text) != len(results[j].Text) {
			return len(results[i].Text) < len(results[j].Text)
		}
		return results[i].Text < results[j].Text
	})

	return results
}
