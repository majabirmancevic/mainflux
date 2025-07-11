// Copyright (c) Mainflux
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"

	"github.com/MainfluxLabs/mainflux/cli"
	sdk "github.com/MainfluxLabs/mainflux/pkg/sdk/go"
	"github.com/spf13/cobra"
)

const defURL string = "http://localhost"

func main() {
	msgContentType := string(sdk.CTJSONSenML)
	sdkConf := sdk.Config{
		AuthURL:         fmt.Sprintf("%s/svcauth", defURL),
		ThingsURL:       fmt.Sprintf("%s/svcthings", defURL),
		WebhooksURL:     fmt.Sprintf("%s/svcwebhooks", defURL),
		UsersURL:        fmt.Sprintf("%s/svcusers", defURL),
		ReaderURL:       fmt.Sprintf("%s/reader", defURL),
		HTTPAdapterURL:  fmt.Sprintf("%s/http", defURL),
		BootstrapURL:    defURL,
		CertsURL:        defURL,
		MsgContentType:  sdk.ContentType(msgContentType),
		TLSVerification: false,
	}

	// Root
	var rootCmd = &cobra.Command{
		Use: "mainfluxlabs-cli",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			sdkConf.MsgContentType = sdk.ContentType(msgContentType)
			s := sdk.NewSDK(sdkConf)
			cli.SetSDK(s)
		},
	}

	// API commands
	healthCmd := cli.NewHealthCmd()
	usersCmd := cli.NewUsersCmd()
	thingsCmd := cli.NewThingsCmd()
	groupsCmd := cli.NewGroupsCmd()
	profilesCmd := cli.NewProfilesCmd()
	webhooksCmd := cli.NewWebhooksCmd()
	orgsCmd := cli.NewOrgsCmd()
	orgMembershipsCmd := cli.NewOrgMembershipsCmd()
	groupMembershipsCmd := cli.NewGroupMembershipsCmd()
	messagesCmd := cli.NewMessagesCmd()
	provisionCmd := cli.NewProvisionCmd()
	certsCmd := cli.NewCertsCmd()
	keysCmd := cli.NewKeysCmd()

	// Root Commands
	rootCmd.AddCommand(healthCmd)
	rootCmd.AddCommand(usersCmd)
	rootCmd.AddCommand(groupsCmd)
	rootCmd.AddCommand(thingsCmd)
	rootCmd.AddCommand(profilesCmd)
	rootCmd.AddCommand(webhooksCmd)
	rootCmd.AddCommand(orgsCmd)
	rootCmd.AddCommand(orgMembershipsCmd)
	rootCmd.AddCommand(groupMembershipsCmd)
	rootCmd.AddCommand(messagesCmd)
	rootCmd.AddCommand(provisionCmd)
	rootCmd.AddCommand(certsCmd)
	rootCmd.AddCommand(keysCmd)

	// Root Flags
	rootCmd.PersistentFlags().StringVarP(
		&sdkConf.AuthURL,
		"auth-url",
		"a",
		sdkConf.AuthURL,
		"Auth service URL",
	)

	rootCmd.PersistentFlags().StringVarP(
		&sdkConf.CertsURL,
		"certs-url",
		"c",
		sdkConf.CertsURL,
		"Certs service URL",
	)

	rootCmd.PersistentFlags().StringVarP(
		&sdkConf.ThingsURL,
		"things-url",
		"t",
		sdkConf.ThingsURL,
		"Things service URL",
	)

	rootCmd.PersistentFlags().StringVarP(
		&sdkConf.WebhooksURL,
		"webhooks-url",
		"w",
		sdkConf.WebhooksURL,
		"Webhooks service URL",
	)

	rootCmd.PersistentFlags().StringVarP(
		&sdkConf.UsersURL,
		"users-url",
		"u",
		sdkConf.UsersURL,
		"Users service URL",
	)

	rootCmd.PersistentFlags().StringVarP(
		&sdkConf.HTTPAdapterURL,
		"http-url",
		"p",
		sdkConf.HTTPAdapterURL,
		"HTTP adapter URL",
	)

	rootCmd.PersistentFlags().StringVarP(
		&msgContentType,
		"content-type",
		"y",
		msgContentType,
		"Message content type",
	)

	rootCmd.PersistentFlags().BoolVarP(
		&sdkConf.TLSVerification,
		"insecure",
		"i",
		sdkConf.TLSVerification,
		"Do not check for TLS cert",
	)

	rootCmd.PersistentFlags().BoolVarP(
		&cli.RawOutput,
		"raw",
		"r",
		cli.RawOutput,
		"Enables raw output mode for easier parsing of output",
	)

	// Client and Profiles Flags
	rootCmd.PersistentFlags().UintVarP(
		&cli.Limit,
		"limit",
		"l",
		100,
		"Limit query parameter",
	)

	rootCmd.PersistentFlags().UintVarP(
		&cli.Offset,
		"offset",
		"o",
		0,
		"Offset query parameter",
	)

	rootCmd.PersistentFlags().StringVarP(
		&cli.Name,
		"name",
		"n",
		"",
		"Name query parameter",
	)

	rootCmd.PersistentFlags().StringVarP(
		&cli.Email,
		"email",
		"e",
		"",
		"User email query parameter",
	)

	rootCmd.PersistentFlags().StringVarP(
		&cli.Metadata,
		"metadata",
		"m",
		"",
		"Metadata query parameter",
	)

	rootCmd.PersistentFlags().StringVarP(
		&cli.Format,
		"format",
		"f",
		"",
		"Message format query parameter",
	)

	rootCmd.PersistentFlags().StringVarP(
		&cli.Subtopic,
		"subtopic",
		"s",
		"",
		"Subtopic query parameter",
	)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
