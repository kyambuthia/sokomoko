package main

import "fmt"

type commandError struct {
	Command   string
	Operation string
	Err       error
}

func (e *commandError) Error() string {
	return fmt.Sprintf("%s: %s: %v", e.Command, e.Operation, e.Err)
}

func (e *commandError) Unwrap() error {
	return e.Err
}

func wrapCommandError(command, operation string, err error) error {
	if err == nil {
		return nil
	}
	return &commandError{
		Command:   command,
		Operation: operation,
		Err:       err,
	}
}
