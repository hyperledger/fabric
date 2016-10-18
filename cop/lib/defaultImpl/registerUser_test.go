package defaultImpl

import "testing"

// type User struct {
// 	enrollID    string
// 	enrollPwd   []byte
// 	affiliation string
// 	attributes  []*pb.Attribute
// }
//
// var (
// 	testAdmin = User{enrollID: "admin", enrollPwd: []byte("Xurw3yU9zI0l")}
// 	testUser  = User{enrollID: "testUser", affiliation: "institution_a", attributes: []*pb.Attribute{&pb.Attribute{Name: "roles", Value: []byte("peer,validator")}}}
// )

func TestRegisterUserRegistrar(t *testing.T) {
	server := NewServer()
	_, err := server.registerUser(testUser.enrollID, testUser.affiliation, testUser.attributes, testAdmin.enrollID)
	if err != nil {
		t.Error("Failed to register user with registrar ", err)
	}
}

func TestRegisterUserNonRegistrar(t *testing.T) {
	server := NewServer()
	_, err := server.registerUser(testUser.enrollID, testUser.affiliation, testUser.attributes, testUser.enrollID)
	if err == nil {
		t.Error("Should failed to register with non-registrat user ", err)
	}
}
