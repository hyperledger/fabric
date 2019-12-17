package grouper

import (
	"fmt"

	"github.com/tedsuo/ifrit"
)

/*
A Member associates a unique name with a Runner.
*/
type Member struct {
	Name string
	ifrit.Runner
}

/*
Members are treated as an ordered list. Member names must be unique.
*/
type Members []Member

/*
Validate checks that all member names in the list are unique. It returns an
error of type ErrDuplicateNames if duplicates are detected.
*/
func (m Members) Validate() error {
	foundNames := map[string]struct{}{}
	foundToken := struct{}{}
	duplicateNames := []string{}

	for _, member := range m {
		_, present := foundNames[member.Name]
		if present {
			duplicateNames = append(duplicateNames, member.Name)
			continue
		}
		foundNames[member.Name] = foundToken
	}

	if len(duplicateNames) > 0 {
		return ErrDuplicateNames{duplicateNames}
	}
	return nil
}

/*
ErrDuplicateNames is returned to indicate two or more members with the same name
were detected. Because more than one duplicate name may be detected in a single
pass, ErrDuplicateNames contains a list of all duplicate names found.
*/
type ErrDuplicateNames struct {
	DuplicateNames []string
}

func (e ErrDuplicateNames) Error() string {
	var msg string

	switch len(e.DuplicateNames) {
	case 0:
		msg = fmt.Sprintln("ErrDuplicateNames initialized without any duplicate names.")
	case 1:
		msg = fmt.Sprintln("Duplicate member name:", e.DuplicateNames[0])
	default:
		msg = fmt.Sprintln("Duplicate member names:")
		for _, name := range e.DuplicateNames {
			msg = fmt.Sprintln(name)
		}
	}

	return msg
}
