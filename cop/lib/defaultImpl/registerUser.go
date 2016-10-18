package defaultImpl

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/cloudflare/cfssl/log"
	pb "github.com/hyperledger/fabric/cop/protos"
	"github.com/spf13/viper"
)

const (
	roles  string = "roles"
	peer   string = "peer"
	client string = "client"
)

func (s *Server) registerUser(id string, affiliation string, attributes []*pb.Attribute, registrar string) (string, error) {
	log.Debugf("Received request to register user with id: %s, affiliation: %s, role: %s, registrar: %s\n",
		id, affiliation, attributes, registrar)

	var enrollID, tok string
	var err error

	if registrar != "" {
		// Check the permissions of member named 'registrar' to perform this registration
		err = s.canRegister(registrar, attributes)
		if err != nil {
			return "", err
		}
	}

	enrollID, err = s.validateAndGenerateEnrollID(id, affiliation, attributes)
	if err != nil {
		return "", err
	}
	tok, err = s.registerUserWithEnrollID(id, enrollID, attributes)
	if err != nil {
		return "", err
	}
	return tok, nil
}

func (s *Server) validateAndGenerateEnrollID(id, affiliation string, attr []*pb.Attribute) (string, error) {
	// Check whether the affiliation is required for the current user.

	// Affiliation is required if the role is client or peer.
	// Affiliation is not required if the role is validator or auditor.
	if s.requireAffiliation(attr) {
		valid, err := s.isValidAffiliation(affiliation)
		if err != nil {
			return "", err
		}

		if !valid {
			return "", errors.New("Invalid affiliation group " + affiliation)
		}

		return s.generateEnrollID(id, affiliation)
	}

	return "", nil
}

func (s *Server) generateEnrollID(id string, affiliation string) (string, error) {
	if id == "" || affiliation == "" {
		return "", errors.New("Please provide all the input parameters, id and role")
	}

	if strings.Contains(id, "\\") || strings.Contains(affiliation, "\\") {
		return "", errors.New("Do not include the escape character \\ as part of the values")
	}

	return id + "\\" + affiliation, nil
}

// registerUserWithEnrollID registers a new user and its enrollmentID, role and state
func (s *Server) registerUserWithEnrollID(id string, enrollID string, attr []*pb.Attribute) (string, error) {
	mutex.Lock()
	defer mutex.Unlock()

	log.Debug("Registering user: ", id)

	var tok string
	tok = randomString(12)

	// TODO: Update db with registered user

	return tok, nil
}

func (s *Server) isValidAffiliation(affiliation string) (bool, error) {
	log.Debug("Validating affiliation: " + affiliation)
	// Check cop.yaml to see if affiliation is valid
	// TODO: Check this from database after bootstrapping has occured

	affiliations := viper.GetStringMapString("security.affiliations")
	for name := range affiliations {
		newKey := "security.affiliations" + "." + name
		groups := viper.GetStringMapString(newKey)
		for g := range groups {
			newKey2 := newKey + "." + g
			aff := viper.GetStringSlice(newKey2)
			for _, a := range aff {
				if a == affiliation {
					return true, nil
				}
			}
		}

	}

	return false, nil
}

func (s *Server) requireAffiliation(attributes []*pb.Attribute) bool {
	log.Debug("requireAffiliation, attributes: ", attributes)

	for _, attr := range attributes {
		values := string(attr.Value)
		valuesArray := strings.Split(values, ",")
		if attr.Name == "roles" {
			for _, value := range valuesArray {
				if value == peer || value == client {
					return true
				}
			}
		}
	}

	return false
}

type MemberMetadata struct {
	Registrar Registrar `json:"registrar"`
}

type Registrar struct {
	Roles         []string `json:"roles"`
	DelegateRoles []string `json:"delegateRoles"`
}

func (s *Server) canRegister(registrar string, attributes []*pb.Attribute) error {
	log.Debug("registrar: ", registrar)
	// var registrarMetadataStr string
	// TODO: Call db to get metadata for registrar. Right now reading from YAML

	registrarMetadata, err := isRegistrar(registrar)
	if err != nil {
		return errors.New("Can't Register: " + err.Error())
	}

	for _, attr := range attributes {
		if attr.Name == roles {
			values := string(attr.Value)
			valuesArray := strings.Split(values, ",")
			for _, r := range registrarMetadata.Registrar.Roles {
				for _, value := range valuesArray {
					if value == r {
						return nil
					}
				}
			}
		}
		return errors.New("Can't Register")
	}
	return nil
}

func isRegistrar(registrar string) (*MemberMetadata, error) {
	log.Debugf("Check if specified registrar (%s) has appropriate permissions", registrar)
	users := viper.GetStringMapString("security.users")
	var memberMetadata string
	for user, flds := range users {
		if user == registrar {
			vals := strings.Fields(flds)
			if len(vals) >= 4 {
				memberMetadata = vals[3]
			}
		}
	}

	if memberMetadata == "" {
		return nil, errors.New("No registrar found by name: " + registrar)
	}

	memberMetadata = removeQuotes(memberMetadata)
	var mm MemberMetadata
	err := json.Unmarshal([]byte(memberMetadata), &mm)
	if err != nil {
		log.Errorf("newMemberMetadata: error: %s, metadata: %s\n", err.Error(), mm)
	}
	return &mm, err
}
