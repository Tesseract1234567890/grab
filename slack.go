package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/slack-go/slack/socketmode"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"net/http"

    "github.com/akamensky/argparse"
)

type grabSubCmd int

const (
    help grabSubCmd = iota
    app // append, but append is a keyword IG
    rng // range, but range is a keyword IG
    summarize
)

func slackBot() {
	go func() {
		for evt := range client.Events {
			switch evt.Type {
			case socketmode.EventTypeHello:
				fmt.Println("Greetings!")
			case socketmode.EventTypeConnecting:
				fmt.Println("Connecting to Slack with Socket Mode...")
			case socketmode.EventTypeConnectionError:
				fmt.Println("Connection failed. Retrying later...")
			case socketmode.EventTypeConnected:
				fmt.Println("Connected to Slack with Socket Mode.")
			case socketmode.EventTypeEventsAPI:
				eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					fmt.Printf("Ignored %+v\n", evt)
					continue
				}
				fmt.Printf("Event received: %+v\n", eventsAPIEvent)
				client.Ack(*evt.Request)
				switch eventsAPIEvent.Type {
				case slackevents.CallbackEvent:
					innerEvent := eventsAPIEvent.InnerEvent
					switch ev := innerEvent.Data.(type) {
					case *slackevents.AppMentionEvent:
						handleMention(ev) // Deal with commands
					}
				default:
					client.Debugf("unsupported Events API event received")
				}
			case socketmode.EventTypeInteractive:
				callback, ok := evt.Data.(slack.InteractionCallback)
				if !ok {
					fmt.Printf("Ignored %+v\n", evt)
					continue
				}
				fmt.Printf("Interaction received: %+v\n", callback)
				var payload interface{}

				switch callback.Type {
				case slack.InteractionTypeBlockActions:
					handleInteraction(&evt, &callback) // Deal with subsequent interactions from users
				default:

				}
				client.Ack(*evt.Request, payload)
			default:
				fmt.Fprintf(os.Stderr, "Unexpected event type received: %s\n", evt.Type)
			}
		}
	}()

	client.Run()
}

type Command struct {
	clobberFlag *bool
	summarizeFlag *bool
	appendOpts AppendCmd
	rangeOpts RangeCmd 
}

type AppendCmd struct {
	title *string
	section *string
}

type RangeCmd struct {
	firstMessage *string
	lastMessage *string
	title *string
	section *string
}

func interpretCommand(tokenizedCommand []string) (command Command) {
	parser := argparse.NewParser("grab", "A command-line tool for grabbing content")

	command.clobberFlag = parser.Flag("c", "clobber", &argparse.Options{Help: "Overwrite possibly existing content"})
	command.summarizeFlag = parser.Flag("s", "summarize", &argparse.Options{Help: "Summarize content"})

    // @grab <chom> skz
	appendCmd := parser.NewCommand("append", "Append this thread as new content to the wiki.")
	command.appendOpts.title = appendCmd.StringPositional(&argparse.Options{Required: true, Help: "Title"})
	command.appendOpts.section = appendCmd.StringPositional(&argparse.Options{Required: false, Help: "Section"})

    // @grab range <start> <end> chom skz
	rangeCmd := parser.NewCommand("range", "Append messages between the given links to the wiki, inclusive")
	command.rangeOpts.firstMessage = rangeCmd.StringPositional(&argparse.Options{Required: true, Help: "First chronological message to be saved"})
	command.rangeOpts.lastMessage = rangeCmd.StringPositional(&argparse.Options{Required: true, Help: "Last chronological message to be saved"})
	command.rangeOpts.title = rangeCmd.StringPositional(&argparse.Options{Required: false, Help: "Title"})
	command.rangeOpts.section = rangeCmd.StringPositional(&argparse.Options{Required: false, Help: "Section"})

	parser.Parse(tokenizedCommand)
	return command
}

// Code to run if someone mentions the bot.
func handleMention(ev *slackevents.AppMentionEvent) {
	commandMessage := "chom"
	subCommand := "skz"
	if false { 
		// If someone @grab's in a thread, that implies that they want to save the entire contents of the thread.
		// Get every message in the thread, and create a new wiki page with a transcription.

		_, possibleTitle, possibleSectionTitle, transcript := packageConversation(ev.Channel, ev.ThreadTimeStamp)

		// Now that we have the final title, check if the article exists
		newArticleURL, missing, err := getArticleURL(possibleTitle)
		if err != nil {
			fmt.Println(err)
		}

		if missing {
			// Post ephemeral message to user
			_, err = client.PostEphemeral(
				ev.Channel, 
				ev.User, 
				slack.MsgOptionTS(ev.ThreadTimeStamp), 
				slack.MsgOptionText(
					`That article doesn't exist. Try again without the "append" subcommand.`,
					false,
				),
			)
			if err != nil {
				fmt.Printf("failed posting message: %v", err)
			}
			return
		}

		// Only check this if a section title was provided.
		if len(possibleSectionTitle) > 0 && len(commandMessage) >= 4 {
			sectionExists, err := sectionExists(possibleTitle, possibleSectionTitle)
			if err != nil {
				fmt.Println(err)
			}

			if !sectionExists {
				// Post ephemeral message to user
				_, err = client.PostEphemeral(
					ev.Channel, 
					ev.User, 
					slack.MsgOptionTS(ev.ThreadTimeStamp), 
					slack.MsgOptionText(
						`That section doesn't exist. Try again without the "append" subcommand.`,
						false,
					),
				)
				if err != nil {
					fmt.Printf("failed posting message: %v", err)
				}
				return
			}
		} else {
			// Necessary because my shitty code sets it automatically but here
			// we don't necessarily want that. I could add another bool to the
			// packageConversation function but that's work and effort and I am
			// verly lazy :)
			possibleSectionTitle = ""
		}

		err = publishToWiki(true, possibleTitle, possibleSectionTitle, transcript)
		if err != nil {
			fmt.Println(err)
		}

		baseResponse := "Article updated! You can find it at: "
		newArticleURL, _, err = getArticleURL(possibleTitle)
		if err != nil {
			fmt.Println(err)
		}

		// Post ephemeral message to user
		_, err = client.PostEphemeral(ev.Channel, ev.User, slack.MsgOptionTS(ev.ThreadTimeStamp), slack.MsgOptionText(fmt.Sprintf("%s %s", baseResponse, newArticleURL), false))
		if err != nil {
			fmt.Printf("failed posting message: %v", err)
		}
		
	} else if subCommand == "range" {
	} else if subCommand == "summarize" {
		// TODO: Funny AI shit
	} else if subCommand == "help" {

	} else { // Default behavior
		// If someone @grab's in a thread, that implies that they want to save the entire contents of the thread.
		// Get every message in the thread, and create a new wiki page with a transcription.

        // First off, check if the command message has a title. If we've made
        // it this far, it'll be the second token passed.



        // === old ===

		_, possibleTitle, possibleSectionTitle, transcript := packageConversation(ev.Channel, ev.ThreadTimeStamp)

		// Now that we have the final title, check if the article exists
		newArticleURL, missing, err := getArticleURL(possibleTitle)
		if err != nil {
			fmt.Println(err)
		}

		// If the title doesn't check out, check if the section does. If not,
		// then scream.
		sectionExists, err := sectionExists(possibleTitle, possibleSectionTitle)
		if err != nil {
			fmt.Println(err)
		}
		// If there is an existing article with that title
		if !missing && (len(possibleSectionTitle) > 0 && sectionExists) {
			// Ask user if they _really_ want to overwrite the page
			warningMessage := fmt.Sprintf("A wiki article with this title already exists! (%s) Are you sure you want to *COMPLETELY OVERWRITE IT?*", newArticleURL)
			confirmButton := slack.NewButtonBlockElement(
				"confirm_wiki_page_overwrite",
				"CONFIRM",
				slack.NewTextBlockObject("plain_text", "CONFIRM", false, false),
			)
			confirmButton.Style = "danger"
			blockMsg := slack.MsgOptionBlocks(
				slack.NewSectionBlock(
					slack.NewTextBlockObject(
						"mrkdwn",
						warningMessage,
						false,
						false,
					),
					nil,
					nil,
				),
				slack.NewActionBlock(
					"",
					confirmButton,
					slack.NewButtonBlockElement(
						"cancel_wiki_page_overwrite",
						"CANCEL",
						slack.NewTextBlockObject("plain_text", "CANCEL", false, false),
					),
				),
			)
			_, err := api.PostEphemeral(
				ev.Channel,
				ev.User,
				slack.MsgOptionTS(ev.ThreadTimeStamp),
				blockMsg,
			)
			if err != nil {
				log.Printf("Failed to send message: %v", err)
			}
		} else {
			// If there is no article with that title, then
			// go ahead and publish it, then send the user
			// an ephemeral message of success
			err = publishToWiki(false, possibleTitle, possibleSectionTitle, transcript)
			if err != nil {
				fmt.Println(err)
			}

			baseResponse := "Article saved! You can find it posted at: "
			newArticleURL, _, err := getArticleURL(possibleTitle)
			if err != nil {
				fmt.Println(err)
			}

			// Post ephemeral message to user
			_, err = client.PostEphemeral(ev.Channel, ev.User, slack.MsgOptionTS(ev.ThreadTimeStamp), slack.MsgOptionText(fmt.Sprintf("%s %s", baseResponse, newArticleURL), false))
			if err != nil {
				fmt.Printf("failed posting message: %v", err)
			}
		}
	}
}

func handleInteraction(evt *socketmode.Event, callback *slack.InteractionCallback) {
	actionID := callback.ActionCallback.BlockActions[0].ActionID
	if actionID == "confirm_wiki_page_overwrite" {
		client.Ack(*evt.Request) // Tell Slack we got him

		_, possibleTitle, _, transcript := packageConversation(callback.Container.ChannelID, callback.Container.ThreadTs)

		// Save the transcript to the wiki
		err := publishToWiki(false, possibleTitle, "", transcript) // TODO: No section title? probably bug
		if err != nil {
			fmt.Println(err)
		}

		// Update the ephemeral message
		newArticleURL, _, err := getArticleURL(possibleTitle)
		responseData := fmt.Sprintf(
			`{"replace_original": "true", "thread_ts": "%d", "text": "Article updated! You can find it posted at: %s"}`,
			callback.Container.ThreadTs,
			newArticleURL,
		)
		reader := strings.NewReader(responseData)
		_, err = http.Post(callback.ResponseURL, "application/json", reader)

		if err != nil {
			log.Printf("Failed updating message: %v", err)
		}
	} else if actionID == "cancel_wiki_page_overwrite" {
		client.Ack(*evt.Request) // Tell Slack we got him
		// Update the ephemeral message
		responseData := fmt.Sprintf(
			`{"replace_original": "true", "thread_ts": "%d", "text": "Grab request cancelled."}`,
			callback.Container.ThreadTs,
		)
		reader := strings.NewReader(responseData)
		_, err := http.Post(callback.ResponseURL, "application/json", reader)

		if err != nil {
			log.Printf("Failed updating message: %v", err)
		}
	} else {
		log.Printf("Unexpected Action Occured: %s.\n", actionID, callback.BlockID)
	}
}

func tokenizeCommand(commandMessage string) (tokenizedCommandMessage []string) {
	r := regexp.MustCompile(`\"[^\"]+\"|\S+`)
	return r.FindAllString(commandMessage, -1)
}

func packageConversation(channelID string, threadTs string) (commandMessage []string, possibleTitle string, possibleSectionTitle string, transcript string) {
	// Get the conversation history
	params := slack.GetConversationRepliesParameters{
		ChannelID: channelID,
		Timestamp: threadTs,
	}
	messages, _, _, err := api.GetConversationReplies(&params)
	if err != nil {
		fmt.Println(err)
	}

	// Theoretically, the last message in the convo should have the title,
	// if any was submitted
	commandMessage = tokenizeCommand(messages[len(messages)-1].Text)
	// Make sure the title exists, and also isn't mistaken for a subcomamnd
	subCommands := map[string]bool{"append": true, "range": true, "help": true, "summarize": true}
	lookahead := 0
	if len(commandMessage) > 2 && subCommands[commandMessage[1]] {
		lookahead = 1
	}
	if len(commandMessage) > 1 + lookahead && !subCommands[commandMessage[1 + lookahead]] {
		possibleTitle = strings.Trim(commandMessage[1 + lookahead], `\"`) // I think the tokenizer leaves the quotes.
	}
	// While we're at it, check for a sectionTitle
	if len(commandMessage) > 2 + lookahead && !subCommands[commandMessage[2 + lookahead]] {
		possibleSectionTitle = strings.Trim(commandMessage[2 + lookahead], `\"`) // I think the tokenizer leaves the quotes.
	}

	// Generate a wiki-friendly transcript of the conversation
	var genTitle string
	genTitle, transcript = generateTranscript(messages)
	// Get a title if we need one.
	if len(possibleTitle) == 0 {
		possibleTitle = genTitle
	}
	if len(possibleSectionTitle) == 0 {
		possibleSectionTitle = genTitle
	}

	return commandMessage, possibleTitle, possibleSectionTitle, transcript
}

// Takes in a slack thread and...
// Gets peoples' CSH usernames and makes them into page links (TODO)
// Removes any mention of Grab
// Adds human readable timestamp to the top of the transcript
// Formats nicely
// Fetches images, uploads them to the Wiki, and links them in appropriately (TODO)
func generateTranscript(conversation []slack.Message) (title string, transcript string) {
	// Define the desired format layout
	timeLayout := "2006-01-02 at 15:04"
	currentTime := time.Now().Format(timeLayout)

	transcript += "Conversation begins at " + currentTime + "\n\n"

	// Remove any message sent by Grab
	// Call the AuthTest method to check the authentication and retrieve the bot's user ID
	authTestResponse, err := api.AuthTest()
	if err != nil {
		log.Fatalf("Error calling AuthTest: %s", err)
	}

	// Print the bot's user ID
	fmt.Printf("Current Time: %s\n", currentTime)
	fmt.Printf("Bot UserID: %s\n", authTestResponse.UserID)

	// Remove messages sent by Grab	and mentioning Grab
	// Format conversation into string line-by-line
	fmt.Printf("Looking for: <@%s>\n", authTestResponse.UserID)
	var pureConversation []slack.Message
	conversationUsers := map[string]string {}
	for _, message := range conversation {
		if message.User != authTestResponse.UserID && !strings.Contains(message.Text, fmt.Sprintf("<@%s>", authTestResponse.UserID)) {
			pureConversation = append(pureConversation, message)
			
			// Translate the user id to a user name
			var msgUser *slack.User	
			if len(conversationUsers[message.User]) == 0 {
				msgUser, err = api.GetUserInfo(message.User)
				if err != nil {
					fmt.Println(err)
				} else {
					conversationUsers[message.User] = msgUser.Name
				}
			}
			var msgUserName string
			msgUserName = conversationUsers[message.User]

			transcript += msgUserName + ": " + message.Text + "\n\n"
			fmt.Printf("[%s] %s: %s\n", message.Timestamp, message.User, message.Text)
		}
	}

	return pureConversation[0].Text, transcript
}

