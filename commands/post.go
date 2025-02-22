// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package commands

import (
	"encoding/json"
	"fmt"

	"github.com/mattermost/mattermost-server/v6/model"

	"github.com/mattermost/mmctl/v6/client"
	"github.com/mattermost/mmctl/v6/printer"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var PostCmd = &cobra.Command{
	Use:   "post",
	Short: "Management of posts",
}

var PostCreateCmd = &cobra.Command{
	Use:     "create",
	Short:   "Create a post",
	Example: `  post create myteam:mychannel --message "some text for the post"`,
	Args:    cobra.ExactArgs(1),
	RunE:    withClient(postCreateCmdF),
}

var PostListCmd = &cobra.Command{
	Use:   "list",
	Short: "List posts for a channel",
	Example: `  post list myteam:mychannel
  post list myteam:mychannel --number 20`,
	Args: cobra.ExactArgs(1),
	RunE: withClient(postListCmdF),
}

func init() {
	PostCreateCmd.Flags().StringP("message", "m", "", "Message for the post")
	PostCreateCmd.Flags().StringP("reply-to", "r", "", "Post id to reply to")

	PostListCmd.Flags().IntP("number", "n", 20, "Number of messages to list")
	PostListCmd.Flags().BoolP("show-ids", "i", false, "Show posts ids")
	PostListCmd.Flags().BoolP("follow", "f", false, "Output appended data as new messages are posted to the channel")

	PostCmd.AddCommand(
		PostCreateCmd,
		PostListCmd,
	)

	RootCmd.AddCommand(PostCmd)
}

func postCreateCmdF(c client.Client, cmd *cobra.Command, args []string) error {
	message, _ := cmd.Flags().GetString("message")
	if message == "" {
		return errors.New("message cannot be empty")
	}

	replyTo, _ := cmd.Flags().GetString("reply-to")
	if replyTo != "" {
		replyToPost, _, err := c.GetPost(replyTo, "")
		if err != nil {
			return err
		}
		if replyToPost.RootId != "" {
			replyTo = replyToPost.RootId
		}
	}

	channel := getChannelFromChannelArg(c, args[0])
	if channel == nil {
		return errors.New("Unable to find channel '" + args[0] + "'")
	}

	post := &model.Post{
		ChannelId: channel.Id,
		Message:   message,
		RootId:    replyTo,
	}

	url := "/posts" + "?set_online=false"
	data, err := post.ToJSON()
	if err != nil {
		return fmt.Errorf("could not decode post: %w", err)
	}

	if _, err := c.DoAPIPost(url, data); err != nil {
		return fmt.Errorf("could not create post: %s", err.Error())
	}
	return nil
}

func eventDataToPost(eventData map[string]interface{}) (*model.Post, error) {
	post := &model.Post{}
	var rawPost string
	for k, v := range eventData {
		if k == "post" {
			rawPost = v.(string)
		}
	}

	err := json.Unmarshal([]byte(rawPost), &post)
	if err != nil {
		return nil, err
	}
	return post, nil
}

func printPost(c client.Client, post *model.Post, usernames map[string]string, showIds bool) {
	var username string

	if usernames[post.UserId] != "" {
		username = usernames[post.UserId]
	} else {
		user, _, err := c.GetUser(post.UserId, "")
		if err != nil {
			username = post.UserId
		} else {
			usernames[post.UserId] = user.Username
			username = user.Username
		}
	}

	if showIds {
		printer.PrintT(fmt.Sprintf("\u001b[31m%s\u001b[0m \u001b[34;1m[%s]\u001b[0m {{.Message}}", post.Id, username), post)
	} else {
		printer.PrintT(fmt.Sprintf("\u001b[34;1m[%s]\u001b[0m {{.Message}}", username), post)
	}
}

func postListCmdF(c client.Client, cmd *cobra.Command, args []string) error {
	printer.SetSingle(true)

	channel := getChannelFromChannelArg(c, args[0])
	if channel == nil {
		return errors.New("Unable to find channel '" + args[0] + "'")
	}

	number, _ := cmd.Flags().GetInt("number")
	showIds, _ := cmd.Flags().GetBool("show-ids")
	follow, _ := cmd.Flags().GetBool("follow")

	postList, _, err := c.GetPostsForChannel(channel.Id, 0, number, "", false)
	if err != nil {
		return err
	}

	posts := postList.ToSlice()
	usernames := map[string]string{}
	for i := 1; i <= len(posts); i++ {
		post := posts[len(posts)-i]
		printPost(c, post, usernames, showIds)
	}

	if follow {
		ws, err := InitWebSocketClient()
		if err != nil {
			return err
		}

		appErr := ws.Connect()
		if appErr != nil {
			return errors.New(appErr.Error())
		}

		ws.Listen()
		for {
			event := <-ws.EventChannel
			if event.EventType() == model.WebsocketEventPosted {
				post, err := eventDataToPost(event.GetData())
				if err != nil {
					fmt.Println("Error parsing incoming post: " + err.Error())
				}
				if post.ChannelId == channel.Id {
					printPost(c, post, usernames, showIds)
				}
			}
		}
	}
	return nil
}
