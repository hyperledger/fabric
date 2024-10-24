/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policydsl

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/Knetic/govaluate"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	mb "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"google.golang.org/protobuf/proto"
)

// Gate values
const (
	GateAnd   = "And"
	GateOr    = "Or"
	GateOutOf = "OutOf"
)

// Role values for principals
const (
	RoleAdmin   = "admin"
	RoleMember  = "member"
	RoleClient  = "client"
	RolePeer    = "peer"
	RoleOrderer = "orderer"
)

var (
	regex = regexp.MustCompile(
		fmt.Sprintf("^([[:alnum:].-]+)([.])(%s|%s|%s|%s|%s)$",
			RoleAdmin, RoleMember, RoleClient, RolePeer, RoleOrderer),
	)
	regexErr = regexp.MustCompile("^No parameter '([^']+)' found[.]$")
)

// a stub function - it returns the same string as it's passed.
// This will be evaluated by second/third passes to convert to a proto policy
func outof(args ...interface{}) (interface{}, error) {
	toret := "outof("

	if len(args) < 2 {
		return nil, fmt.Errorf("expected at least two arguments to NOutOf. Given %d", len(args))
	}

	arg0 := args[0]
	// govaluate treats all numbers as float64 only. But and/or may pass int/string. Allowing int/string for flexibility of caller
	if n, ok := arg0.(float64); ok {
		toret += strconv.Itoa(int(n))
	} else if n, ok := arg0.(int); ok {
		toret += strconv.Itoa(n)
	} else if n, ok := arg0.(string); ok {
		toret += n
	} else {
		return nil, fmt.Errorf("unexpected type %s", reflect.TypeOf(arg0))
	}

	for _, arg := range args[1:] {
		toret += ", "

		switch t := arg.(type) {
		case string:
			if regex.MatchString(t) {
				toret += "'" + t + "'"
			} else {
				toret += t
			}
		default:
			return nil, fmt.Errorf("unexpected type %s", reflect.TypeOf(arg))
		}
	}

	return toret + ")", nil
}

func and(args ...interface{}) (interface{}, error) {
	args = append([]interface{}{len(args)}, args...)
	return outof(args...)
}

func or(args ...interface{}) (interface{}, error) {
	args = append([]interface{}{1}, args...)
	return outof(args...)
}

// firstPass processes a variadic list of arguments and returns a formatted string.
// The function expects arguments to be of either string, float32, or float64 types.
func firstPass(args ...interface{}) (interface{}, error) {
	toret := "outof(ID"
	for _, arg := range args {
		toret += ", "

		switch t := arg.(type) {
		case string:
			if regex.MatchString(t) {
				toret += "'" + t + "'"
			} else {
				toret += t
			}
		case float32:
		case float64:
			toret += strconv.Itoa(int(t))
		default:
			return nil, fmt.Errorf("unexpected type %s", reflect.TypeOf(arg))
		}
	}

	return toret + ")", nil
}

// secondPass processes a list of arguments to build a "t-out-of-n" policy.
// It expects the first argument to be a context, the second an integer (threshold t),
// and the rest as either principals (strings) or pre-existing policies.
func secondPass(args ...interface{}) (interface{}, error) {
	/* general sanity check, we expect at least 3 args */
	if len(args) < 3 {
		return nil, fmt.Errorf("at least 3 arguments expected, got %d", len(args))
	}

	/* get the first argument, we expect it to be the context */
	var ctx *context
	switch v := args[0].(type) {
	case *context:
		ctx = v
	default:
		return nil, fmt.Errorf("unrecognized type, expected the context, got %s", reflect.TypeOf(args[0]))
	}

	/* get the second argument, we expect an integer telling us
	   how many of the remaining we expect to have*/
	var t int
	switch arg := args[1].(type) {
	case float64:
		t = int(arg)
	default:
		return nil, fmt.Errorf("unrecognized type, expected a number, got %s", reflect.TypeOf(args[1]))
	}

	/* get the n in the t out of n */
	n := len(args) - 2

	/* sanity check - t should be positive, permit equal to n+1, but disallow over n+1 */
	if t < 0 || t > n+1 {
		return nil, fmt.Errorf("invalid t-out-of-n predicate, t %d, n %d", t, n)
	}

	policies := make([]*cb.SignaturePolicy, 0)

	/* handle the rest of the arguments */
	for _, principal := range args[2:] {
		switch t := principal.(type) {
		/* if it's a string, we expect it to be formed as
		   <MSP_ID> . <ROLE>, where MSP_ID is the MSP identifier
		   and ROLE is either a member, an admin, a client, a peer or an orderer*/
		case string:
			/* split the string */
			subm := regex.FindAllStringSubmatch(t, -1)
			if subm == nil || len(subm) != 1 || len(subm[0]) != 4 {
				return nil, fmt.Errorf("error parsing principal %s", t)
			}

			/* get the right role */
			var r mb.MSPRole_MSPRoleType

			switch subm[0][3] {
			case RoleMember:
				r = mb.MSPRole_MEMBER
			case RoleAdmin:
				r = mb.MSPRole_ADMIN
			case RoleClient:
				r = mb.MSPRole_CLIENT
			case RolePeer:
				r = mb.MSPRole_PEER
			case RoleOrderer:
				r = mb.MSPRole_ORDERER
			default:
				return nil, fmt.Errorf("error parsing role %s", t)
			}

			/* build the principal we've been told */
			mspRole, err := proto.Marshal(&mb.MSPRole{MspIdentifier: subm[0][1], Role: r})
			if err != nil {
				return nil, fmt.Errorf("error marshalling msp role: %s", err)
			}

			p := &mb.MSPPrincipal{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal:               mspRole,
			}
			ctx.principals = append(ctx.principals, p)

			/* create a SignaturePolicy that requires a signature from
			   the principal we've just built*/
			dapolicy := SignedBy(int32(ctx.IDNum))
			policies = append(policies, dapolicy)

			/* increment the identity counter. Note that this is
			   suboptimal as we are not reusing identities. We
			   can deduplicate them easily and make this puppy
			   smaller. For now it's fine though */
			// TODO: deduplicate principals
			ctx.IDNum++

		/* if we've already got a policy we're good, just append it */
		case *cb.SignaturePolicy:
			policies = append(policies, t)

		default:
			return nil, fmt.Errorf("unrecognized type, expected a principal or a policy, got %s", reflect.TypeOf(principal))
		}
	}

	return NOutOf(int32(t), policies), nil
}

type context struct {
	IDNum      int
	principals []*mb.MSPPrincipal
}

func newContext() *context {
	return &context{IDNum: 0, principals: make([]*mb.MSPPrincipal, 0)}
}

// FromString takes a string representation of the policy,
// parses it and returns a SignaturePolicyEnvelope that
// implements that policy. The supported language is as follows:
//
// GATE(P[, P])
//
// where:
//   - GATE is either "and" or "or"
//   - P is either a principal or another nested call to GATE
//
// A principal is defined as:
//
// # ORG.ROLE
//
// where:
//   - ORG is a string (representing the MSP identifier)
//   - ROLE takes the value of any of the RoleXXX constants representing
//     the required role
func FromString(policy string) (*cb.SignaturePolicyEnvelope, error) {
	// first we translate the and/or business into outof gates
	intermediate, err := govaluate.NewEvaluableExpressionWithFunctions(
		policy, map[string]govaluate.ExpressionFunction{
			GateAnd:                    and,
			strings.ToLower(GateAnd):   and,
			strings.ToUpper(GateAnd):   and,
			GateOr:                     or,
			strings.ToLower(GateOr):    or,
			strings.ToUpper(GateOr):    or,
			GateOutOf:                  outof,
			strings.ToLower(GateOutOf): outof,
			strings.ToUpper(GateOutOf): outof,
		},
	)
	if err != nil {
		return nil, err
	}

	intermediateRes, err := intermediate.Evaluate(map[string]interface{}{})
	if err != nil {
		// attempt to produce a meaningful error
		if regexErr.MatchString(err.Error()) {
			sm := regexErr.FindStringSubmatch(err.Error())
			if len(sm) == 2 {
				return nil, fmt.Errorf("unrecognized token '%s' in policy string", sm[1])
			}
		}

		return nil, err
	}

	resStr, ok := intermediateRes.(string)
	if !ok {
		return nil, fmt.Errorf("invalid policy string '%s'", policy)
	}

	// we still need two passes. The first pass just adds an extra
	// argument ID to each of the outof calls. This is
	// required because govaluate has no means of giving context
	// to user-implemented functions other than via arguments.
	// We need this argument because we need a global place where
	// we put the identities that the policy requires
	exp, err := govaluate.NewEvaluableExpressionWithFunctions(
		resStr,
		map[string]govaluate.ExpressionFunction{"outof": firstPass},
	)
	if err != nil {
		return nil, err
	}

	res, err := exp.Evaluate(map[string]interface{}{})
	if err != nil {
		// attempt to produce a meaningful error
		if regexErr.MatchString(err.Error()) {
			sm := regexErr.FindStringSubmatch(err.Error())
			if len(sm) == 2 {
				return nil, fmt.Errorf("unrecognized token '%s' in policy string", sm[1])
			}
		}

		return nil, err
	}

	resStr, ok = res.(string)
	if !ok {
		return nil, fmt.Errorf("invalid policy string '%s'", policy)
	}

	ctx := newContext()
	parameters := make(map[string]interface{}, 1)
	parameters["ID"] = ctx

	exp, err = govaluate.NewEvaluableExpressionWithFunctions(
		resStr,
		map[string]govaluate.ExpressionFunction{"outof": secondPass},
	)
	if err != nil {
		return nil, err
	}

	res, err = exp.Evaluate(parameters)
	if err != nil {
		// attempt to produce a meaningful error
		if regexErr.MatchString(err.Error()) {
			sm := regexErr.FindStringSubmatch(err.Error())
			if len(sm) == 2 {
				return nil, fmt.Errorf("unrecognized token '%s' in policy string", sm[1])
			}
		}

		return nil, err
	}

	rule, ok := res.(*cb.SignaturePolicy)
	if !ok {
		return nil, fmt.Errorf("invalid policy string '%s'", policy)
	}

	p := &cb.SignaturePolicyEnvelope{
		Identities: ctx.principals,
		Version:    0,
		Rule:       rule,
	}

	return p, nil
}
