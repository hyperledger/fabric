/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cauthdsl

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"

	"github.com/Knetic/govaluate"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
)

var regex *regexp.Regexp = regexp.MustCompile("^([[:alnum:]]+)([.])(member|admin)$")
var regexErr *regexp.Regexp = regexp.MustCompile("^No parameter '([^']+)' found[.]$")

func and(args ...interface{}) (interface{}, error) {
	toret := "outof(" + strconv.Itoa(len(args))
	for _, arg := range args {
		toret += ", "
		switch t := arg.(type) {
		case string:
			if regex.MatchString(t) {
				toret += "'" + t + "'"
			} else {
				toret += t
			}
		default:
			return nil, fmt.Errorf("Unexpected type %s", reflect.TypeOf(arg))
		}
	}

	return toret + ")", nil
}

func or(args ...interface{}) (interface{}, error) {
	toret := "outof(" + strconv.Itoa(1)
	for _, arg := range args {
		toret += ", "
		switch t := arg.(type) {
		case string:
			if regex.MatchString(t) {
				toret += "'" + t + "'"
			} else {
				toret += t
			}
		default:
			return nil, fmt.Errorf("Unexpected type %s", reflect.TypeOf(arg))
		}
	}

	return toret + ")", nil
}

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
			return nil, fmt.Errorf("Unexpected type %s", reflect.TypeOf(arg))
		}
	}

	return toret + ")", nil
}

func secondPass(args ...interface{}) (interface{}, error) {
	/* general sanity check, we expect at least 3 args */
	if len(args) < 3 {
		return nil, fmt.Errorf("At least 3 arguments expected, got %d", len(args))
	}

	/* get the first argument, we expect it to be the context */
	var ctx *context
	switch v := args[0].(type) {
	case *context:
		ctx = v
	default:
		return nil, fmt.Errorf("Unrecognized type, expected the context, got %s", reflect.TypeOf(args[0]))
	}

	/* get the second argument, we expect an integer telling us
	   how many of the remaining we expect to have*/
	var t int
	switch arg := args[1].(type) {
	case float64:
		t = int(arg)
	default:
		return nil, fmt.Errorf("Unrecognized type, expected a number, got %s", reflect.TypeOf(args[1]))
	}

	/* get the n in the t out of n */
	var n int = len(args) - 1

	/* sanity check - t better be <= n */
	if t > n {
		return nil, fmt.Errorf("Invalid t-out-of-n predicate, t %d, n %d", t, n)
	}

	policies := make([]*common.SignaturePolicy, 0)

	/* handle the rest of the arguments */
	for _, principal := range args[2:] {
		switch t := principal.(type) {
		/* if it's a string, we expect it to be formed as
		   <MSP_ID> . <ROLE>, where MSP_ID is the MSP identifier
		   and ROLE is either a member of an admin*/
		case string:
			/* split the string */
			subm := regex.FindAllStringSubmatch(t, -1)
			if subm == nil || len(subm) != 1 || len(subm[0]) != 4 {
				return nil, fmt.Errorf("Error parsing principal %s", t)
			}

			/* get the right role */
			var r msp.MSPRole_MSPRoleType
			if subm[0][3] == "member" {
				r = msp.MSPRole_MEMBER
			} else {
				r = msp.MSPRole_ADMIN
			}

			/* build the principal we've been told */
			p := &msp.MSPPrincipal{
				PrincipalClassification: msp.MSPPrincipal_ROLE,
				Principal:               utils.MarshalOrPanic(&msp.MSPRole{MspIdentifier: subm[0][1], Role: r})}
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
		case *common.SignaturePolicy:
			policies = append(policies, t)

		default:
			return nil, fmt.Errorf("Unrecognized type, expected a principal or a policy, got %s", reflect.TypeOf(principal))
		}
	}

	return NOutOf(int32(t), policies), nil
}

type context struct {
	IDNum      int
	principals []*msp.MSPPrincipal
}

func newContext() *context {
	return &context{IDNum: 0, principals: make([]*msp.MSPPrincipal, 0)}
}

// FromString takes a string representation of the policy,
// parses it and returns a SignaturePolicyEnvelope that
// implements that policy. The supported language is as follows
//
// GATE(P[, P])
//
// where
//	- GATE is either "and" or "or"
//	- P is either a principal or another nested call to GATE
//
// a principal is defined as
//
// ORG.ROLE
//
// where
//	- ORG is a string (representing the MSP identifier)
//	- ROLE is either the string "member" or the string "admin" representing the required role
func FromString(policy string) (*common.SignaturePolicyEnvelope, error) {
	// first we translate the and/or business into outof gates
	intermediate, err := govaluate.NewEvaluableExpressionWithFunctions(policy, map[string]govaluate.ExpressionFunction{"AND": and, "and": and, "OR": or, "or": or})
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

	// we still need two passes. The first pass just adds an extra
	// argument ID to each of the outof calls. This is
	// required because govaluate has no means of giving context
	// to user-implemented functions other than via arguments.
	// We need this argument because we need a global place where
	// we put the identities that the policy requires
	exp, err := govaluate.NewEvaluableExpressionWithFunctions(intermediateRes.(string), map[string]govaluate.ExpressionFunction{"outof": firstPass})
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

	ctx := newContext()
	parameters := make(map[string]interface{}, 1)
	parameters["ID"] = ctx

	exp, err = govaluate.NewEvaluableExpressionWithFunctions(res.(string), map[string]govaluate.ExpressionFunction{"outof": secondPass})
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

	p := &common.SignaturePolicyEnvelope{
		Identities: ctx.principals,
		Version:    0,
		Rule:       res.(*common.SignaturePolicy),
	}

	return p, nil
}
