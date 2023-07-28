package bmsteams

/*
Dieser Code importiert verschiedene Pakete und Bibliotheken
für die Entwicklung einer Matterbridge-Brücke,
die Microsoft Graph-API verwendet, um eine Verbindung
zu Microsoft Teams herzustellen und zu kommunizieren.
*/
import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/42wim/matterbridge/bridge"
	"github.com/42wim/matterbridge/bridge/config"
	"github.com/davecgh/go-spew/spew"
	msgraph "github.com/yaegashi/msgraph.go/beta"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/mattn/godown"
	//msgraph "github.com/yaegashi/msgraph.go/beta"
	//"github.com/yaegashi/msgraph.go/msauth"
	//"github.com/microsoftgraph/msgraph-sdk-go"
	//"golang.org/x/oauth2/clientcredentials"
	//"golang.org/x/net/context"
)

/*
Dieser Code definiert zwei Variablen: eine Liste von Standardberechtigungen
für den Microsoft Graph-API-Zugriff und ein regulärer Ausdruck, der verwendet wird,
um Anhänge aus einer Zeichenfolge zu entfernen, indem er nach einem bestimmten Muster sucht.
*/
var (
	defaultScopes = []string{"openid", "profile", "offline_access", "Group.Read.All", "Group.ReadWrite.All", "ChannelMessage.ReadWrite"}
	attachRE      = regexp.MustCompile(`<attachment id=.*?attachment>`)
)

/*
Dieser Code definiert eine Struktur namens "Bmsteams", die Konfigurationsdaten für die Verbindung
mit der Microsoft Teams-API speichert und Funktionen für die Verwendung
innerhalb einer Matterbridge-Brücke bereitstellt.
*/
// type Bmsteams struct {
// 	gc    *msgraph.GraphServiceRequestBuilder // -> inoffiziellen yaegashi-Bibliothek
// 	ctx   context.Context
// 	botID string
// 	*bridge.Config
// 	idsForDelMap map[string]string
// }

type Bmsteams struct {
	gc    *msgraphsdk.GraphServiceClient // -> offizielle Microsoft Graph API
	ctx   context.Context
	botID string
	*bridge.Config
	idsForDelMap map[string]string
}

func New(cfg *bridge.Config) bridge.Bridger {
	return &Bmsteams{Config: cfg, idsForDelMap: make(map[string]string)}
}

type teamsMessageInfo struct {
	mTime   time.Time //Zeitstempel
	replies map[string]time.Time
}

// inoffiziellen yaegashi Connect-Methode

// func (b *Bmsteams) Connect() error {
// 	tokenCachePath := b.GetString("sessionFile")
// 	if tokenCachePath == "" {
// 		tokenCachePath = "msteams_session.json"
// 	}
// 	ctx := context.Background()
// 	m := msauth.NewManager()  // -> inoffiziellen yaegashi-Bibliothek
// 	m.LoadFile(tokenCachePath) //nolint:errcheck
// 	ts, err := m.DeviceAuthorizationGrant(ctx, b.GetString("TenantID"), b.GetString("ClientID"), defaultScopes, nil) // "m" wir von yaegashi genutzt
// 	if err != nil {
// 		return err
// 	}
// 	err = m.SaveFile(tokenCachePath) // "m" wir von yaegashi genutzt
// 	if err != nil {
// 		b.Log.Errorf("Couldn't save sessionfile in %s: %s", tokenCachePath, err)
// 	}
// 	// make file readable only for matterbridge user
// 	err = os.Chmod(tokenCachePath, 0o600)
// 	if err != nil {
// 		b.Log.Errorf("Couldn't change permissions for %s: %s", tokenCachePath, err)
// 	}
// 	httpClient := oauth2.NewClient(ctx, ts)
// 	graphClient := msgraph.NewClient(httpClient) // -> inoffiziellen yaegashi-Bibliothek
// 	//graphClient := msgraph.NewGraphServiceRequest(httpClient) -> offizielle Microsoft Graph API
// 	b.gc = graphClient // "graphClient" wir von yaegashi genutzt
// 	b.ctx = ctx

// 	err = b.setBotID()
// 	if err != nil {
// 		return err
// 	}
// 	b.Log.Info("Connection succeeded")
// 	return nil
// }

// offizielle msgraph API
func (b *Bmsteams) Connect() error {
	tokenCachePath := b.GetString("sessionFile")
	if tokenCachePath == "" {
		tokenCachePath = "msteams_session.json"
	}
	ctx := context.Background()

	clientID := b.GetString("ClientID")
	clientSecret := b.GetString("ClientSecret")
	tenantID := b.GetString("TenantID")
	scopes := defaultScopes
	//scopes := []string{"https://graph.microsoft.com/.default"},

	// Set up the OAuth2 configuration
	conf := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/token",
		Scopes:       scopes,
	}

	// Create an http.Client from the config
	httpClient := conf.Client(ctx)

	// Create a new Microsoft Graph client
	graphClient := msgraphsdk.NewGraphServiceClient("https://graph.microsoft.com/v1.0", httpClient)

	// Assign the graphClient to b.gc
	b.gc = graphClient
	b.ctx = ctx

	err := b.setBotID()
	if err != nil {
		return err
	}
	b.Log.Info("Connection succeeded")
	return nil
}

//-----------------------------

func (b *Bmsteams) Disconnect() error {
	return nil
}

func (b *Bmsteams) JoinChannel(channel config.ChannelInfo) error {
	go func(name string) {
		for {
			err := b.poll(name)
			if err != nil {
				b.Log.Errorf("polling failed for %s: %s. retrying in 5 seconds", name, err)
			}
			time.Sleep(time.Second * 5)
		}
	}(channel.Name)
	return nil
}

func (b *Bmsteams) DeleteToTeams(msg config.Message) (string, error) {

	// wenn msg eine parentID hat: msg.Id und ParentId mittels der map mappen -> zweite Version des Request ausführen
	// an sonsten: nur msg.ID mappen und erste Version ausführen
	//Die ganze if-else muss angepasst werden nutzt den call von yaegashi
	// if msg.ParentID == "" {
	// 	err := b.gc.Teams().ID(b.GetString("TeamID")).Channels().ID(msg.Channel).Messages().ID(msg.ID).Request().JSONRequest(b.ctx, "POST", "/softDelete", nil, nil)
	// 	if err != nil {
	// 		return "", err
	// 	}
	// } else {
	// 	err := b.gc.Teams().ID(b.GetString("TeamID")).Channels().ID(msg.Channel).Messages().ID(msg.ParentID).Replies().ID(msg.ID).Request().JSONRequest(b.ctx, "POST", "/softDelete", nil, nil)
	// 	if err != nil {
	// 		return "", err
	// 	}
	// }
	// return "", nil

	//https://learn.microsoft.com/en-us/graph/api/chatmessage-softdelete?view=graph-rest-beta&tabs=go
	//_, err := b.gc.Me().Team().ChannelsById(channelID).MessagesById(messageID).Request().Delete(b.ctx)

	if msg.ParentID == "" {
		err := b.gc.Teams().ByTeamId(b.GetString("TeamID")).Channels().ByChannelId(msg.Channel).Message().ByMessageId(msg.ID).SoftDelete().Post(b.ctx, nil)
		if err != nil {
			return "", err
		}
	} else {
		err := b.gc.Teams().ByTeamId(b.GetString("TeamID")).Channels().ByChannelId(msg.Channel).Messages().ByMessageId(msg.ParentID).Replies().ByReplieId(msg.ID).SoftDelete().Post(b.ctx, nil)
		if err != nil {
			return "", err
		}
	}
	return "", nil
}

// func für bearbeiten und für replies bearbeiten:
//	1. erkennen ob config.Message eine neue ist oder ein Edit (-> wahrscheinlich daran ob ID gesetzt ist oder nicht)
//  2. API calls herausfinden
//		b.gc.Teams().ID(b.GetString("TeamID")).Channels().ID(msg.Channel).Messages().ID(msg.ParentID)[ .Replies().ID(msg.ID) ].Request().Update(...)
//		- herausfinden ob beim edit in dem ChatMessage-Objekt noch die ID stehen muss
//  3. entweder code für Mentions und Bilder in einzelne Funktionen ausgliedern um sie auch im Edit-Fall zu verwenden
//		oder: in Send und SendReply nur den jeweiligen API-Aufruf anhand der Situation auswählen und die restlichen code-Pfade beibehalten

// func (b *Bmsteams) processingMentionText(msg config.Message) (string, error) {
// 	// convert to HTML
// 	formatUsername := "<strong>" + msg.Username + "</strong>\n"
// 	htmlText := "<p>" + formatUsername + "</p>\n\n" + msg.Text
// 	//htmlText = strings.Replace(htmlText, "\n", "<br>", -1)

// 	// process mentions
// 	var chatMessageMentionsArr []msgraph.ChatMessageMention //array für mention-objekte
// 	//xxx := msgraph.ChatMessageMention{Mentioned: msgraph.IdentitySet{User: msgraph.Identity{ID: "...", DisplayName: "...."}}}
// 	mentionPattern := regexp.MustCompile(`(?:^|\s)@([^@\s]+)`)
// 	mentionCounter := 0

// 	htmlText = mentionPattern.ReplaceAllStringFunc(htmlText, func(matchingMention string) string {
// 		mentionCounter++
// 		//TODO: entscheiden ob channel/all oder user-mention erzeugt werden muss
// 		//msgraph.chatMessageMention{Mentioned: msgraph.IdentitySet{Conversation: "channel"}}
// 		mentionCounterPointer := mentionCounter
// 		channelIDTeams := msg.Channel
// 		chanelName := "PublicTest5"
// 		if matchingMention == "channel" || matchingMention == "all" {
// 			//channelIdentityType := msgraph.ChatMessageMention{Mentioned: &msgraph.IdentitySet{Conversation: &msgraph.ConversationIdentity{IdentityTypeConversation: msgraph.ConversationIdentityTypePChannel}}}
// 			mentionContent := msgraph.ChatMessageMention{
// 				ID:          &mentionCounterPointer,
// 				MentionText: &matchingMention,
// 				Mentioned: &msgraph.IdentitySet{
// 					Conversation: &msgraph.ConversationIdentity{
// 						IdentityTypeConversation: msgraph.ConversationIdentityTypePChannel,
// 						Identity: &msgraph.Identity{
// 							ID:          &channelIDTeams,
// 							DisplayName: &chanelName,
// 						},
// 					},
// 				},
// 			}
// 			chatMessageMentionsArr = append(chatMessageMentionsArr, mentionContent)
// 		} else {
// 			mentionContentAll := msgraph.ChatMessageMention{
// 				ID:          &mentionCounterPointer,
// 				MentionText: &matchingMention,
// 				Mentioned: &msgraph.IdentitySet{
// 					Conversation: &msgraph.ConversationIdentity{
// 						IdentityTypeConversation: msgraph.ConversationIdentityTypePChannel,
// 						Identity: &msgraph.Identity{
// 							ID:          &channelIDTeams,
// 							DisplayName: &chanelName,
// 						},
// 					},
// 				},
// 			}
// 			chatMessageMentionsArr = append(chatMessageMentionsArr, mentionContentAll)

// 		}
// 		return fmt.Sprintf("<at id=\"%v\">%s</at>", mentionCounter, matchingMention)
// 	})
// }

// func (b *Bmsteams) processingFile(msg config.Message) (string, error) {
// 	// process attached images
// 	var hostedContentsMessagesArr []msgraph.ChatMessageHostedContent

// 	if msg.Extra["file"] != nil {
// 		for i, file := range msg.Extra["file"] {
// 			fileInfo := file.(config.FileInfo)
// 			b.Log.Debugf("=> Receiving the fileInfo: %#v", fileInfo)
// 			extIndex := strings.LastIndex(fileInfo.Name, ".")
// 			ext := fileInfo.Name[extIndex:]
// 			contentType := mime.TypeByExtension(ext)
// 			if contentType == "image/jpg" || contentType == "image/jpeg" || contentType == "image/png" {
// 				b.Log.Debugf("=> Receiving  the content Type: %#v", contentType)
// 				contentBytes := fileInfo.Data
// 				encodedContentBytes := base64.StdEncoding.EncodeToString(*contentBytes)
// 				temporaryIdCounterInt := i
// 				b.Log.Debugf("=> Receiving  the temporary Id-Counter: %#v", temporaryIdCounterInt)
// 				temporaryIdCounterStr := strconv.Itoa(temporaryIdCounterInt)
// 				tag := "<img src=\"../hostedContents/" + temporaryIdCounterStr + "/$value\">" // break
// 				mdToHtml += tag
// 				b.Log.Debugf("=> Output of the text for body content%#v", mdToHtml)
// 				// Erstellung einer ChatMessageHostedContent-Struktur mit den Werten aus der Schleife
// 				hostedContent := msgraph.ChatMessageHostedContent{
// 					ContentType:               &contentType,
// 					ContentBytes:              &encodedContentBytes,
// 					MicrosoftGraphTemporaryId: &temporaryIdCounterStr,
// 				}

// 				// Hinzufügen der Nachricht zum Array
// 				hostedContentsMessagesArr = append(hostedContentsMessagesArr, hostedContent)
// 			} else {
// 				contentText := fmt.Sprintf("<br>Datei %s wurde entfernt.", fileInfo.Name)
// 				mdToHtml += contentText
// 			}

// 		}
// 	}

//}

func (b *Bmsteams) Send(msg config.Message) (string, error) {
	if msg.Event == config.EventMsgDelete {
		return b.DeleteToTeams(msg)
	}

	b.Log.Debugf("=> Receiving %#v", msg)
	if msg.ParentValid() {
		return b.sendReply(msg)
	}

	// Handle prefix hint for unthreaded messages.
	if msg.ParentNotFound() {
		msg.ParentID = ""
		msg.Text = fmt.Sprintf("[thread]: %s", msg.Text)
	}

	// if  Event:"msg_delete" ja dann lösche die Messages
	// teams id in einer map merken

	b.Log.Debugf("=> Original Text: '%#v'", msg.Text)

	// convert to HTML
	formatUsername := "<strong>" + msg.Username + "</strong>\n"
	htmlText := "<p>" + formatUsername + "</p>\n\n" + msg.Text
	//htmlText = strings.Replace(htmlText, "\n", "<br>", -1)

	// process mentions
	// []msgraph.ChatMessageMention ist yaegashi
	var chatMessageMentionsArr []msgraph.ChatMessageMention //array für mention-objekte
	//xxx := msgraph.ChatMessageMention{Mentioned: msgraph.IdentitySet{User: msgraph.Identity{ID: "...", DisplayName: "...."}}}
	mentionPattern := regexp.MustCompile(`(?:^|\s)@([^@\s]+)`)
	mentionCounter := 0

	htmlText = mentionPattern.ReplaceAllStringFunc(htmlText, func(matchingMention string) string {
		mentionCounter++
		//TODO: entscheiden ob channel/all oder user-mention erzeugt werden muss
		//msgraph.chatMessageMention{Mentioned: msgraph.IdentitySet{Conversation: "channel"}}
		mentionCounterPointer := mentionCounter
		channelIDTeams := msg.Channel
		chanelName := "PublicTest5"
		if matchingMention == "channel" || matchingMention == "all" {
			//channelIdentityType := msgraph.ChatMessageMention{Mentioned: &msgraph.IdentitySet{Conversation: &msgraph.ConversationIdentity{IdentityTypeConversation: msgraph.ConversationIdentityTypePChannel}}}
			mentionContent := msgraph.ChatMessageMention{ // ->  msgraph.ChatMessageMention yaegashi-Bibliothek
				ID:          &mentionCounterPointer,
				MentionText: &matchingMention,
				Mentioned: &msgraph.IdentitySet{ // ->  &msgraph.IdentitySet yaegashi-Bibliothek
					Conversation: &msgraph.ConversationIdentity{ // ->  &msgraph.ConversationIdentity yaegashi-Bibliothek
						IdentityTypeConversation: msgraph.ConversationIdentityTypePChannel, // ->  msgraph.ConversationIdentityTypePChanne yaegashi-Bibliothek
						Identity: &msgraph.Identity{ // ->   &msgraph.Identity yaegashi-Bibliothek
							ID:          &channelIDTeams,
							DisplayName: &chanelName,
						},
					},
				},
			}
			chatMessageMentionsArr = append(chatMessageMentionsArr, mentionContent)
		} else {
			mentionContentAll := msgraph.ChatMessageMention{ // ->   msgraph.ChatMessageMention yaegashi-Bibliothek
				ID:          &mentionCounterPointer,
				MentionText: &matchingMention,
				Mentioned: &msgraph.IdentitySet{ // ->   &msgraph.IdentitySet yaegashi-Bibliothek
					Conversation: &msgraph.ConversationIdentity{ // ->    &msgraph.ConversationIdentity yaegashi-Bibliothek
						IdentityTypeConversation: msgraph.ConversationIdentityTypePChannel, // ->   yaegashi-Bibliothek
						Identity: &msgraph.Identity{ // ->   yaegashi-Bibliothek
							ID:          &channelIDTeams,
							DisplayName: &chanelName,
						},
					},
				},
			}
			chatMessageMentionsArr = append(chatMessageMentionsArr, mentionContentAll)

		}
		return fmt.Sprintf("<at id=\"%v\">%s</at>", mentionCounter, matchingMention)
	})

	b.Log.Debugf("=> Text with mentions: '%#v'", htmlText)

	mdToHtml := makeHTML(htmlText)
	fmt.Println("=> Receiving the HTM String: ", mdToHtml)

	// process attached images
	var hostedContentsMessagesArr []msgraph.ChatMessageHostedContent // ->   yaegashi-Bibliothek
	msgChatMessageID := msg.ID

	if msg.Extra["file"] != nil {
		for i, file := range msg.Extra["file"] {
			fileInfo := file.(config.FileInfo)
			b.Log.Debugf("=> Receiving the fileInfo: %#v", fileInfo)
			extIndex := strings.LastIndex(fileInfo.Name, ".")
			ext := fileInfo.Name[extIndex:]
			contentType := mime.TypeByExtension(ext)
			if contentType == "image/jpg" || contentType == "image/jpeg" || contentType == "image/png" {
				b.Log.Debugf("=> Receiving  the content Type: %#v", contentType)
				contentBytes := fileInfo.Data
				encodedContentBytes := base64.StdEncoding.EncodeToString(*contentBytes)
				temporaryIdCounterInt := i
				b.Log.Debugf("=> Receiving  the temporary Id-Counter: %#v", temporaryIdCounterInt)
				temporaryIdCounterStr := strconv.Itoa(temporaryIdCounterInt)
				tag := "<img src=\"../hostedContents/" + temporaryIdCounterStr + "/$value\">" // break
				mdToHtml += tag
				b.Log.Debugf("=> Output of the text for body content%#v", mdToHtml)
				// Erstellung einer ChatMessageHo stedContent-Struktur mit den Werten aus der Schleife
				hostedContent := msgraph.ChatMessageHostedContent{ // ->   yaegashi-Bibliothek
					ContentType:               &contentType,
					ContentBytes:              &encodedContentBytes,
					MicrosoftGraphTemporaryId: &temporaryIdCounterStr,
				}

				// Hinzufügen der Nachricht zum Array
				hostedContentsMessagesArr = append(hostedContentsMessagesArr, hostedContent)
			} else {
				contentText := fmt.Sprintf("<br>Datei %s wurde entfernt.", fileInfo.Name)
				mdToHtml += contentText
			}

		}
	}

	content := &msgraph.ItemBody{Content: &mdToHtml, ContentType: msgraph.BodyTypePHTML} // ->   yaegashi-Bibliothek
	rmsg := &msgraph.ChatMessage{                                                        // ->   yaegashi-Bibliothek
		Body:           content,
		Mentions:       chatMessageMentionsArr,
		HostedContents: hostedContentsMessagesArr,
	}
	// add new msg
	//ct := b.gc.Teams().ID(b.GetString("TeamID")).Channels().ID(msg.Channel).Messages().Request()
	ct := b.gc.Teams().ByTeamId(b.GetString("TeamID")).Channels().ByChannelId(msg.Channel).Messages().Post(context.Background(), requestBody, nil)
	// Edit msg
	//cte := b.gc.Teams().ID(b.GetString("TeamID")).Channels().ID(msg.Channel).Messages().ID(msg.ID).Request()
	cte := b.gc.Teams().ByTeamId(b.GetString("TeamID")).Channels().ByChannelId(msg.Channel).Messages().ByMessageId(msg.ParentID).Patch(context.Background(), requestBody, nil)

	if msg.ID != "" {
		b.Log.Debug("rmsg update object: ", spew.Sdump(rmsg))
		err := cte.Update(b.ctx, rmsg)
		//err := cte.JSONRequest(b.ctx, "PATCH", "?model=A", rmsg, nil)
		if err != nil {
			return "", err
		}
		return msg.ID, err
	} else {
		res, err := ct.Add(b.ctx, rmsg)
		if err != nil {
			return "", err
		}
		b.idsForDelMap[msgChatMessageID] = *res.ID
		return *res.ID, nil
	}

}

func (b *Bmsteams) sendReply(msg config.Message) (string, error) {

	// Handle prefix hint for unthreaded messages.
	if msg.ParentNotFound() {
		msg.ParentID = ""
		msg.Text = fmt.Sprintf("[thread]: %s", msg.Text)
	}

	b.Log.Debug("=> Original reply Text: '%s'", msg.Text)

	// convert to HTML
	formatUsername := "<strong> " + msg.Username + "</strong>\n"
	htmlReplyText := "<p>" + formatUsername + "</p>\n\n" + msg.Text

	var chatReplyMessageMentionArr []msgraph.ChatMessageMention // ->   yaegashi-Bibliothek
	replyMentionPattern := regexp.MustCompile(`(?:^|\s)@([^@\s]+)`)
	mentionReplyCounter := 0

	htmlReplyText = replyMentionPattern.ReplaceAllStringFunc(htmlReplyText, func(matchingReplyMention string) string {
		mentionReplyCounter++
		mentionReplyCounterPointer := mentionReplyCounter
		channelIDTeams := msg.Channel
		channelDisplayName := "PublicTest5"
		if matchingReplyMention == "channel" || matchingReplyMention == "all" {
			replyMentionContent := msgraph.ChatMessageMention{ // ->   yaegashi-Bibliothek
				ID:          &mentionReplyCounterPointer,
				MentionText: &matchingReplyMention,
				Mentioned: &msgraph.IdentitySet{ // ->   yaegashi-Bibliothek
					Conversation: &msgraph.ConversationIdentity{ // ->   yaegashi-Bibliothek
						IdentityTypeConversation: msgraph.ConversationIdentityTypePChannel,
						Identity: &msgraph.Identity{
							ID:          &channelIDTeams,
							DisplayName: &channelDisplayName,
						},
					},
				},
			}
			chatReplyMessageMentionArr = append(chatReplyMessageMentionArr, replyMentionContent)
		} else {
			replyMentionContent := msgraph.ChatMessageMention{ // ->   yaegashi-Bibliothek
				ID:          &mentionReplyCounterPointer,
				MentionText: &matchingReplyMention,
				Mentioned: &msgraph.IdentitySet{
					Conversation: &msgraph.ConversationIdentity{
						IdentityTypeConversation: msgraph.ConversationIdentityTypePChannel,
						Identity: &msgraph.Identity{
							ID:          &channelIDTeams,
							DisplayName: &channelDisplayName,
						},
					},
				},
			}
			chatReplyMessageMentionArr = append(chatReplyMessageMentionArr, replyMentionContent)
		}
		return fmt.Sprintf(" <at id=\"%v\">%s</at>", mentionReplyCounter, matchingReplyMention)

	})
	b.Log.Debugf("=> Text with mentions: '%#v'", htmlReplyText)

	mdToHtml := makeHTML(htmlReplyText)
	fmt.Println("=> Receiving the HTM String: ", mdToHtml)

	var hostedContentsMessagesArr []msgraph.ChatMessageHostedContent // ->   yaegashi-Bibliothek

	if msg.Extra["file"] != nil {
		for i, file := range msg.Extra["file"] {
			fileInfo := file.(config.FileInfo)
			b.Log.Debugf("=> Receiving the fileInfo: %#v", fileInfo)
			extIndex := strings.LastIndex(fileInfo.Name, ".")
			ext := fileInfo.Name[extIndex:]
			contentType := mime.TypeByExtension(ext)
			if contentType == "image/jpg" || contentType == "image/jpeg" || contentType == "image/png" {
				b.Log.Debugf("=> Receiving  the content Type: %#v", contentType)
				contentBytes := fileInfo.Data
				encodedContentBytes := base64.StdEncoding.EncodeToString(*contentBytes)
				temporaryIdCounterInt := i
				b.Log.Debugf("=> Receiving  the temporary Id-Counter: %#v", temporaryIdCounterInt)
				temporaryIdCounterStr := strconv.Itoa(temporaryIdCounterInt)
				tag := "<img src=\"../hostedContents/" + temporaryIdCounterStr + "/$value\">"
				mdToHtml += tag
				b.Log.Debugf("=> Output of the text for body content%#v", mdToHtml)
				// Erstellung einer ChatMessageHostedContent-Struktur mit den Werten aus der Schleife
				message := msgraph.ChatMessageHostedContent{ // ->   yaegashi-Bibliothek
					ContentType:               &contentType,
					ContentBytes:              &encodedContentBytes,
					MicrosoftGraphTemporaryId: &temporaryIdCounterStr,
				}

				// Hinzufügen der Nachricht zum Array
				hostedContentsMessagesArr = append(hostedContentsMessagesArr, message)
			} else {
				contentText := fmt.Sprintf("<br>Datei %s wurde entfernt.", fileInfo.Name)
				mdToHtml += contentText
			}
		}
	}

	content := &msgraph.ItemBody{Content: &mdToHtml, ContentType: msgraph.BodyTypePHTML} // ->   yaegashi-Bibliothek
	rmsg := &msgraph.ChatMessage{                                                        // ->   yaegashi-Bibliothek
		Body:           content,
		Mentions:       chatReplyMessageMentionArr,
		HostedContents: hostedContentsMessagesArr,
	}

	// add new reply msg
	//ct := b.gc.Teams().ID(b.GetString("TeamID")).Channels().ID(msg.Channel).Messages().ID(msg.ParentID).Replies().Request()
	ct := b.gc.Teams().ByTeamId(b.GetString("TeamID")).Channels().ByChannelId(msg.Channel).Messages().ByMessageId(msg.ParentID).Replies().ByReplieId(msg.ID).Post(context.Background(), requestBody, nil)
	// Edit  reply msg
	//cte := b.gc.Teams().ID(b.GetString("TeamID")).Channels().ID(msg.Channel).Messages().ID(msg.ParentID).Replies().ID(msg.ID).Request()
	cte := b.gc.Teams().ByTeamId(b.GetString("TeamID")).Channels().ByChannelId(msg.Channel).Messages().ByMessageId(msg.ParentID).Replies().ByReplieId(msg.ID).Patch(context.Background(), requestBody, nil)

	if msg.ID != "" {
		err := cte.Update(b.ctx, rmsg)
		if err != nil {
			return "", err
		}
		return msg.ParentID, err
	} else {
		res, err := ct.Add(b.ctx, rmsg)
		if err != nil {
			return "", err
		}
		b.idsForDelMap[msg.ID] = *res.ID
		return *res.ID, nil
	}

}

func (b *Bmsteams) getMessages(channel string) ([]msgraph.ChatMessage, error) { // ->   yaegashi-Bibliothek
	//ct := b.gc.Teams().ID(b.GetString("TeamID")).Channels().ID(channel).Messages().Request()
	ct := b.gc.Teams().ByTeamId(b.GetString("TeamID")).Channels().ByChannelId(channel).Messages().Post(context.Background(), requestBody, nil)
	ct.Expand("replies")
	rct, err := ct.Get(b.ctx)
	if err != nil {
		return nil, err
	}
	b.Log.Debugf("got %#v messages", len(rct))
	return rct, nil
}

// Verwalten von toplevel map das verwalten der map (finde nachricht, prüfe Zeitstemple von Nachricht, füge Nachrichte ein)

func createMapReplies() map[string]time.Time {
	mapReplies := make(map[string]time.Time)
	return mapReplies

}

func updateMsgToplevel(toplevelMsg *msgraph.ChatMessage, msgToplevelInfo *teamsMessageInfo) { // ->   yaegashi-Bibliothek
	msgToplevelInfo.mTime = *msgTime(toplevelMsg)
}

func updateMsgReplies(msg *msgraph.ChatMessage, msgRepliesInfo *teamsMessageInfo, b *Bmsteams) { // ->   yaegashi-Bibliothek
	mapReplies := createMapReplies()
	for _, reply := range msg.Replies {

		mapReplies[*reply.ID] = *msgTime(&reply)
		// }

	}
	msgRepliesInfo.replies = mapReplies // nur im ersten mal
}

// prüft entweder LastModifiedDateTime oder CreatedDateTime
func msgTime(graphMsg *msgraph.ChatMessage) *time.Time { // ->   yaegashi-Bibliothek
	if graphMsg.LastModifiedDateTime != nil {
		return graphMsg.LastModifiedDateTime
	}

	if graphMsg.DeletedDateTime != nil {
		return graphMsg.DeletedDateTime
	}

	return graphMsg.CreatedDateTime
}

func (b *Bmsteams) skipOwnMessage(msg *msgraph.ChatMessage) bool { // ->   yaegashi-Bibliothek
	if msg.From == nil || msg.From.User == nil {
		return false
	}
	if *msg.From.User.ID == b.botID {
		b.Log.Debug("skipping own message")
		return true // skip own message
	}
	return false // don't skip
}

// func processingConfigMessage(channelName string, msg *msgraph.ChatMessage, b *Bmsteams, options ...func(*config.Message)) {
// 	processingConfigMessage := config.Message{
// 		Username: *msg.From.User.DisplayName,
// 		Channel:  channelName,
// 		Account:  b.Account,
// 		Avatar:   "",
// 		UserID:   *msg.From.User.ID,
// 		ID:       *msg.ID,
// 		Extra:    make(map[string][]interface{}),
// 	}

// 	// Optionale Parameter anwenden
// 	for _, option := range options {
// 		option(&processingConfigMessage)
// 	}

// 	b.Log.Debugf("<= delete toplevel Message is %#v", processingConfigMessage)
// 	b.Remote <- processingConfigMessage
// }

// // optional parameters for the event field
// func WithEvent(event string) func(*config.Message) {
// 	return func(m *config.Message) {
// 		m.Event = config.EventMsgDelete
// 	}
// }

// // optional parameters for the text field
// func WithText(b *Bmsteams, converter func(string) string) func(*config.Message) {
// 	return func(m *config.Message) {
// 		if m.Event == "msg_delete" {
// 			m.Text = "DeleteMe!"
// 		} else {
// 			m.Text = b.convertToMD()
// 		}
// 	}
// }

//nolint:gocognit
func (b *Bmsteams) poll(channelName string) error {
	msgmap := make(map[string]teamsMessageInfo) // Zeitstempel merken für DB
	b.Log.Debug("getting initial messages")
	res, err := b.getMessages(channelName)
	if err != nil {
		return err
	}

	for _, msgToplevel := range res {
		msgToplevelInfo := msgmap[*msgToplevel.ID]
		//msgToplevelInfo.mTime = *msgToplevel.CreatedDateTime  should be done by updateMsgToplevel below
		updateMsgToplevel(&msgToplevel, &msgToplevelInfo)
		updateMsgReplies(&msgToplevel, &msgToplevelInfo, b)
		msgmap[*msgToplevel.ID] = msgToplevelInfo
	}

	// Annahme: botID und msg sind bereits deklariert und initialisiert

	time.Sleep(time.Second * 5)
	b.Log.Debug("polling for messages")
	for {
		res, err := b.getMessages(channelName)

		if err != nil {
			return err
		}
		// check top level messages from oldest to newest
		for i := len(res) - 1; i >= 0; i-- {
			msg := res[i]
			//b.Log.Debugf("\n\n<= toplevel is ID %s", *msg.ID)
			if msgInfo, ok := msgmap[*msg.ID]; ok {
				for _, reply := range msg.Replies {
					//reply.Reactions
					if msgTimeReply, ok := msgInfo.replies[*reply.ID]; ok {
						// timeStamps vergleichen, hat die replies lasmodifdate
						// creattime skip
						//b.Log.Debugf("<= checking reply %s", *reply.ID)
						if msgTimeReply == *msgTime(&reply) {
							//b.Log.Debugf("<= unchanged reply %s", *reply.ID)
							continue

						}

						// changed or deleted reply - update tiome stamp and pass on
						msgInfo.replies[*reply.ID] = *msgTime(&reply)
						if !b.skipOwnMessage(&reply) {
							if reply.DeletedDateTime == nil {
								//time updated for changed reply-ID
								replyText := b.converMentionsAndRemoveHTML(&reply)
								changedReplyObject := config.Message{
									Username: *reply.From.User.DisplayName,
									Text:     replyText,
									Channel:  channelName,
									Account:  b.Account,
									Avatar:   "",
									UserID:   *reply.From.User.ID,
									ID:       *reply.ID,
									ParentID: *msg.ID,
									Extra:    make(map[string][]interface{}),
								}
								b.handleAttachments(&changedReplyObject, reply)
								b.Log.Debugf("<= Updated reply Message ID is %s", *reply.ID)
								b.Remote <- changedReplyObject
							} else {

								deleteReplyObject := config.Message{
									Channel: channelName,
									Text:    "DeleteMe!",
									Account: b.Account,
									Avatar:  "",
									Event:   config.EventMsgDelete,
									ID:      *reply.ID,
								}
								//b.handleAttachments(&deleteReplyObject, msg)
								b.Log.Debugf("<= deleted reply Message is %s", deleteReplyObject)
								b.Remote <- deleteReplyObject
								//delete(msgInfo.replies, replyID)
							}
						}
					} else {

						// new reply
						msgInfo.replies[*reply.ID] = *msgTime(&reply)

						if !b.skipOwnMessage(&reply) {
							replyText := b.converMentionsAndRemoveHTML(&reply)
							newReplyObject := config.Message{
								Username: *reply.From.User.DisplayName,
								Text:     replyText,
								Channel:  channelName,
								Account:  b.Account,
								Avatar:   "",
								UserID:   *reply.From.User.ID,
								ID:       *reply.ID,
								ParentID: *msg.ID,
								Extra:    make(map[string][]interface{}),
							}
							b.handleAttachments(&newReplyObject, reply)
							b.Log.Debugf("<= New reply Message ID is %s", *reply.ID)
							b.Remote <- newReplyObject

						}
					}
				}

				// wenn msg.DeletedDateTime nicht null oder 0(?) ist (also eine Zeit drin steht) dann wurde die TopLevelMSg
				// gelöscht und wir müssen eine entsprechende config.Message schicken um sie auch in mm oder slack zu löschen

				// ------------------------------------------------- //
				if msgInfo.mTime == *msgTime(&msg) {
					continue
				} else {
					msgInfo.mTime = *msgTime(&msg)
				}

				if b.skipOwnMessage(&msg) {
					continue
				}
			} else {
				// toplevel msg is new
				if b.GetBool("debug") {
					b.Log.Debug("Msg dump: ", spew.Sdump(msg))
				}

				msgInfo := teamsMessageInfo{mTime: *msgTime(&msg), replies: make(map[string]time.Time)}
				msgmap[*msg.ID] = msgInfo

				if b.skipOwnMessage(&msg) {
					continue
				}

				// skip non-user message for now.
				if msg.From == nil || msg.From.User == nil {
					continue
				}
			}
			if msg.DeletedDateTime == nil {
				// we did not 'continue' above so this message really needs to be sent
				b.Log.Debugf("<= Sending message from %s on %s to gateway", *msg.From.User.DisplayName, b.Account)
				// extra funktion
				text := b.converMentionsAndRemoveHTML(&msg)
				rmsg := config.Message{
					Username: *msg.From.User.DisplayName,
					Text:     text,
					Channel:  channelName,
					Account:  b.Account,
					Avatar:   "",
					UserID:   *msg.From.User.ID,
					ID:       *msg.ID,
					Extra:    make(map[string][]interface{}),
				}
				b.handleAttachments(&rmsg, msg)
				b.Log.Debugf("<= Message is %#v", rmsg)
				b.Remote <- rmsg
			} else {
				b.Log.Debugf("<= Sending toplevel message from %s on %s to gateway", *msg.From.User.DisplayName, b.Account)
				deletedTopLevelMsg := config.Message{
					Username: *msg.From.User.DisplayName,
					Text:     "DeleteMe!",
					Channel:  channelName,
					Account:  b.Account,
					Avatar:   "",
					UserID:   *msg.From.User.ID,
					ID:       *msg.ID,
					Event:    config.EventMsgDelete,
					Extra:    make(map[string][]interface{}),
				}
				//b.handleAttachments(&deletedTopLevelMsg, msg)
				b.Log.Debugf("<= delete toplevel Message is %#v", deletedTopLevelMsg)
				b.Remote <- deletedTopLevelMsg
				continue
			}

		}
		time.Sleep(time.Second * 5)
	}
}

func (b *Bmsteams) setBotID() error {
	req := b.gc.Me().Request()
	r, err := req.Get(b.ctx)
	if err != nil {
		return err
	}
	b.botID = *r.ID
	return nil
}

// ->   yaegashi-Bibliothek func
func (b *Bmsteams) converMentionsAndRemoveHTML(msg *msgraph.ChatMessage) string {
	// convert mentions in msg.Body.Content

	text := *msg.Body.Content
	mmMentionPattern := regexp.MustCompile(`<at.+?/at>`)
	text = mmMentionPattern.ReplaceAllStringFunc(text, func(matchingMattermostMention string) string {
		//spliten string
		splitString := strings.SplitN(matchingMattermostMention, "\"", 3)
		getIDStr := splitString[1]
		getIDInt, err := strconv.Atoi(getIDStr)
		if err != nil {
			fmt.Println("Error: ", err)
			return ""
		}
		//schleife über alle mentions

		for _, mention := range msg.Mentions {
			if *mention.ID == getIDInt {
				if mention.Mentioned.Conversation != nil {
					return "@" + "channel"
				}
			} else {
				b.Log.Debugf("=> Mention additionalData is empty ")
			}

		}

		return fmt.Sprintf("<at id=\"%v\">%s</at>", getIDStr, matchingMattermostMention)

	})

	withoutHTML := b.convertToMD(text)

	return withoutHTML
}

func (b *Bmsteams) convertToMD(text string) string {
	if !strings.Contains(text, "<div>") {
		return text
	}
	var sb strings.Builder
	err := godown.Convert(&sb, strings.NewReader(text), nil)
	if err != nil {
		b.Log.Errorf("Couldn't convert message to markdown %s", text)
		return text
	}
	return sb.String()
}
