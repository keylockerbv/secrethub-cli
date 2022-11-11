package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/secrethub/secrethub-go/internals/errio"
)

// Errors
var (
	askErr = errio.Namespace("ask")
	// ErrCannotAsk occurs when prompting for input is impossible.
	ErrCannotAsk = askErr.Code("cannot_ask_for_input").Error("Cannot ask for interactive input.\n\n" +
		"This usually happens when you run something non-interactively that needs to ask interactive questions.")
	ErrPassphrasesDoNotMatch = askErr.Code("passphrase_does_not_match").Error("passphrases do not match")
)

// Ask prints out the question and reads the first line of input.
func Ask(io IO, question string) (string, error) {
	r, w, err := io.Prompts()
	if err != nil {
		return "", err
	}

	_, err = fmt.Fprintf(w, "%s", question)
	if err != nil {
		return "", err
	}
	return Readln(r)
}

// AskWithDefault  prints out the question and reads the first line of input.
// If no input is given, the default value is returned.
func AskWithDefault(io IO, question, defaultValue string) (string, error) {
	res, err := Ask(io, fmt.Sprintf("%s [%s] ", question, defaultValue))
	if err != nil {
		return "", err
	}
	if res == "" {
		return defaultValue, nil
	}
	return res, nil
}

// AskSecret prints out the question and reads back the input,
// without echoing it back. Useful for passwords and other sensitive inputs.
func AskSecret(io IO, question string) (string, error) {
	_, promptOut, err := io.Prompts()
	if err != nil {
		return "", err
	}

	_, err = fmt.Fprintf(promptOut, "%s", question)
	if err != nil {
		return "", err
	}

	raw, err := io.ReadSecret()
	if err != nil {
		return "", ErrReadInput(err)
	}

	fmt.Fprintln(promptOut, "")

	return string(raw), nil
}

// AskMultiline prints out the question and reads back the input until an EOF is reached.
// The input is displayed to the user.
func AskMultiline(IO IO, question string) ([]byte, error) {
	promptIn, promptOut, err := IO.Prompts()
	if err != nil {
		return nil, err
	}

	_, err = fmt.Fprintf(promptOut, "%s\n", question)
	if err != nil {
		return nil, err
	}

	raw, err := io.ReadAll(promptIn)
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(promptOut)
	return raw, nil
}

// AskAndValidate asks the user a question and re-prompts the configured amount of times
// when the users answer does not validate.
func AskAndValidate(io IO, question string, n int, validationFunc func(string) error) (string, error) {
	_, promptOut, err := io.Prompts()
	if err != nil {
		return "", err
	}
	for i := 0; i < n; i++ {
		var response string
		response, err = Ask(io, question)
		if err != nil {
			return "", err
		}

		err = validationFunc(response)
		if err == nil {
			return response, nil
		}

		fmt.Fprintf(promptOut, "\nInvalid input: %s\n", err)
		if i != n-1 {
			fmt.Fprintln(promptOut, "Please try again.")
		}
	}
	return "", err
}

// ConfirmCaseInsensitive asks the user to confirm by typing one of the expected strings.
// The comparison is not case-sensitive. If multiple values for expected are given,
// true is returned if the input equals any of the the expected values.
func ConfirmCaseInsensitive(io IO, question string, expected ...string) (bool, error) {
	response, err := Ask(io, fmt.Sprintf("%s: ", question))
	if err != nil {
		return false, err
	}

	response = strings.ToLower(strings.TrimSpace(response))

	for _, e := range expected {
		if response == strings.ToLower(e) {
			return true, nil
		}
	}

	return false, nil
}

// AskPassphrase asks for a password and then asks to type it again for confirmation.
// When the user types two different passphrases, he is asked again. When
// the answers still haven't matched after trying n times, the error
// ErrPassphrasesDoNotMatch is returned. For the empty answer ("") no
// confirmation is asked.
func AskPassphrase(io IO, question string, repeatPhrase string, n int) (string, error) {
	_, promptOut, err := io.Prompts()
	if err != nil {
		return "", err
	}

	for i := 0; i < n; i++ {
		answer, err := AskSecret(io, question)
		if err != nil {
			return "", err
		}

		if answer == "" {
			return answer, nil
		}

		confirmed, err := AskSecret(io, repeatPhrase)
		if err != nil {
			return "", err
		}

		if answer == confirmed {
			return answer, nil
		}
		fmt.Fprintln(promptOut, "Answers do not match. Try again.")
	}
	return "", ErrPassphrasesDoNotMatch
}

// ConfirmationType defines what AskYesNo uses as the default answer.
type ConfirmationType int

const (
	// DefaultNone assumes no default [y/n]
	DefaultNone ConfirmationType = iota
	// DefaultNo assumes no as the default answer [y/N]
	DefaultNo
	// DefaultYes assumes yes as the default answer [Y/n]
	DefaultYes
)

// AskYesNo asks the user for confirmation. A user must type in "yes" or "no" and
// then press enter. It has fuzzy matching, so "y", "Y", "yes", "YES", and "Yes"
// all count as confirmations. If no input is given, it will return true with
// DefaultYes and false with DefaultNo. If the input is not recognized, it will
// ask again. The function retries 3 times. If it still has no valid response
// after that, it returns false.
func AskYesNo(io IO, question string, t ConfirmationType) (bool, error) {
	defaultRetry := 3

	for i := 1; i <= defaultRetry; i++ {
		// After defaultRetry tries we assume a default no
		if i == defaultRetry {
			t = DefaultNo
		}

		yesNo := "y/n"
		if t == DefaultNo {
			yesNo = "y/N"
		} else if t == DefaultYes {
			yesNo = "Y/n"
		}

		response, err := Ask(io, fmt.Sprintf("%s [%s]: ", question, yesNo))
		if err != nil {
			return false, err
		}

		response = strings.ToLower(strings.TrimSpace(response))

		if response == "y" || response == "yes" || (response == "" && t == DefaultYes) {
			return true, nil
		} else if response == "n" || response == "no" || (response == "" && t == DefaultNo) {
			return false, nil
		}
	}

	return false, nil
}

// Choose gives the user the provided options asks them to choose one.
// It returns the index of the option chosen, starting with 0.
func Choose(io IO, question string, options []string, n int) (int, error) {
	_, w, err := io.Prompts()
	if err != nil {
		return 0, err
	}

	_, err = fmt.Fprintf(w, "%s\n", question)
	if err != nil {
		return 0, err
	}

	for i, option := range options {
		fmt.Fprintf(w, "  %d) %s\n", i+1, option)
	}

	parseFunc := func(in string) (int, error) {
		res, err := strconv.Atoi(strings.Trim(in, " )."))
		if err != nil {
			return 0, errors.New("not a valid number")
		}
		if res < 1 || res > len(options) {
			return 0, errors.New("out of bounds")
		}
		return res - 1, nil
	}

	res, err := AskAndValidate(io, "Give the number of an option: ", n, func(option string) error {
		_, err := parseFunc(option)
		return err
	})
	if err != nil {
		return 0, err
	}
	return parseFunc(res)
}

type Option struct {
	Value   string
	Display string
}

func (o Option) String() string {
	return o.Display
}

func ChooseDynamicOptionsValidate(io IO, question string, getOptions func() ([]Option, bool, error), optionName string, validateFunc func(string) error) (string, error) {
	r, w, err := io.Prompts()
	if err != nil {
		return "", err
	}

	if optionName == "" {
		optionName = "option"
	}

	s := selecter{
		r:            r,
		w:            w,
		getOptions:   getOptions,
		question:     question,
		addOwn:       true,
		validateFunc: validateFunc,
		optionName:   optionName,
	}
	return s.run()
}

func ChooseDynamicOptions(io IO, question string, getOptions func() ([]Option, bool, error), addOwn bool, optionName string) (string, error) {
	r, w, err := io.Prompts()
	if err != nil {
		return "", err
	}

	if optionName == "" {
		optionName = "option"
	}

	s := selecter{
		r:          r,
		w:          w,
		getOptions: getOptions,
		question:   question,
		addOwn:     addOwn,
		optionName: optionName,
	}
	return s.run()
}

type selecter struct {
	r            io.Reader
	w            io.Writer
	getOptions   func() ([]Option, bool, error)
	validateFunc func(string) error
	question     string
	addOwn       bool
	optionName   string

	done    bool
	options []Option
}

func (s *selecter) moreOptions() error {
	if s.done {
		fmt.Fprintln(s.w, "No more options available.")
		return nil
	}

	options, done, err := s.getOptions()
	if err != nil {
		return err
	}

	s.done = done
	w := tabwriter.NewWriter(s.w, 0, 4, 4, ' ', 0)
	for i, option := range options {
		fmt.Fprintf(w, "%d) %s\n", len(s.options)+i+1, option)
	}
	s.options = append(s.options, options...)

	err = w.Flush()
	if err != nil {
		return err
	}

	fmt.Fprintf(s.w, "Type the number of an option or type a %s", s.optionName)
	if !s.done {
		fmt.Fprint(s.w, " (press [ENTER] for more options)")
	}
	fmt.Fprintln(s.w, ":")

	return nil
}

func (s *selecter) run() (string, error) {
	fmt.Fprintf(s.w, s.question+" (press [ENTER] for options)\n")
	return s.process()
}

func (s *selecter) process() (string, error) {
	in, err := Readln(s.r)
	if err != nil {
		return "", err
	}

	if in == "" {
		err = s.moreOptions()
		if err != nil {
			return "", err
		}
		return s.process()
	}

	choice, err := strconv.Atoi(in)
	if err != nil || choice < 1 || choice > len(s.options) {
		if s.addOwn {
			if s.validateFunc != nil {
				return in, s.validateFunc(in)
			}
			return in, nil
		}

		_, err = fmt.Fprintf(os.Stderr, "%s is not a valid choice\n", in)
		if err != nil {
			return in, err
		}
		return s.process()
	}

	return s.options[choice-1].Value, nil
}
