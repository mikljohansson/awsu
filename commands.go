package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func filterExistingCredentials(list []string) []string {
	var result []string
	for _, item := range list {
		if !strings.HasPrefix(item, "AWS_ACCESS_KEY_ID=") &&
			!strings.HasPrefix(item, "AWS_SECRET_ACCESS_KEY=") &&
			!strings.HasPrefix(item, "AWS_SESSION_TOKEN=") &&
			!strings.HasPrefix(item, "AWS_DEFAULT_REGION=") {
			result = append(result, item)
		}
	}

	return result
}

// Encrypts data from stdin and writes to stdout
func executeCommand(iamProfile string, durationSeconds int64, args []string) {
	// Human readable name of this session shown in AWS logs, for debugging purposes
	hostname, err := os.Hostname()
	sessionName := fmt.Sprintf("%s-%s-%s",
		defaults(os.Getenv("USER"), "unknown"),
		defaults(hostname, os.Getenv("HOST"), os.Getenv("HOSTNAME"), "unknown"),
		randSeq(8))

	// Initialize the session
	var accessKeyID, secretAccessKey, sessionToken, region string

	// Force enable Shared Config to support $AWS_DEFAULT_REGION and ~/.aws/config profiles
	sess, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})
	check(err, "Failed to initialize the AWS session")

	if iamProfile != "" {
		svc := sts.New(sess)

		// Assume role given by ARN
		params := &sts.AssumeRoleInput{
			RoleArn:         aws.String(iamProfile),  // Required
			RoleSessionName: aws.String(sessionName), // Required
			DurationSeconds: aws.Int64(durationSeconds),
			//		ExternalId:      aws.String("externalIdType"),
			//		Policy:          aws.String("sessionPolicyDocumentType"),
			//		SerialNumber:    aws.String("serialNumberType"),
			//		TokenCode:       aws.String("tokenCodeType"),
		}

		resp, err := svc.AssumeRole(params)
		check(err, "Failed to assume role")

		accessKeyID = *resp.Credentials.AccessKeyId
		secretAccessKey = *resp.Credentials.SecretAccessKey
		sessionToken = *resp.Credentials.SessionToken
		region = *sess.Config.Region
	} else {
		// Output the session credentials
		creds, err := sess.Config.Credentials.Get()
		check(err, "Failed to retrive credentials from session")

		accessKeyID = creds.AccessKeyID
		secretAccessKey = creds.SecretAccessKey
		sessionToken = creds.SessionToken
		region = *sess.Config.Region
	}

	// Default to launch a subshell
	binary := defaults(os.Getenv("SHELL"), "/bin/sh")

	// Resolve absolute path of binary
	if len(args) >= 1 {
		binary, err = exec.LookPath(args[0])
		check(err)
	}

	// Inject the temporary credentials
	env := append(filterExistingCredentials(os.Environ()),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", accessKeyID),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", secretAccessKey))

	if sessionToken != "" {
		env = append(env, fmt.Sprintf("AWS_SESSION_TOKEN=%s", sessionToken))
	}

	if region != "" {
		env = append(env, fmt.Sprintf("AWS_DEFAULT_REGION=%s", region))
	}

	// Execute subcommand
	err = syscall.Exec(binary, args, env)
	check(err)
}
