package bmsteams

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/42wim/matterbridge/bridge/config"
	"github.com/42wim/matterbridge/bridge/helper"

	//msgraph "github.com/yaegashi/msgraph.go/beta"
	graphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// func (b *Bmsteams) findFile(weburl string) (string, error) {
// 	itemRB, err := b.gc.GetDriveItemByURL(b.ctx, weburl)
// 	if err != nil {
// 		return "", err
// 	}
// 	itemRB.Workbook().Worksheets()
// 	b.gc.Workbooks()
// 	item, err := itemRB.Request().Get(b.ctx)
// 	if err != nil {
// 		return "", err
// 	}
// 	if url, ok := item.GetAdditionalData("@microsoft.graph.downloadUrl"); ok {
// 		return url.(string), nil
// 	}
// 	return "", nil
// }

func (b *Bmsteams) findFile(weburl string) (string, error) {
	// Benutzen Sie msgraph-sdk-go um das DriveItem basierend auf weburl zu holen
	//"driveId": "b!JlPIZmueOEWX6r3H27EqMn2Lr-62cHxBgM0HgwraBU7imYHm8qZmS6IQf3W3gkvJ"
	var DriveID *string
	drivesResponse, err := b.gc.Me().Drives().Get(context.Background(), nil)
	if err != nil {
		return "", err
	}
	drives := drivesResponse.GetValue()

	// Drucke die Drive IDs
	for i, drive := range drives {
		if i == 0 { // Wenn es sich um die erste Iteration handelt
			DriveID = drive.GetId()
		}
	}

	if DriveID != nil {
		fmt.Println("ID of the first drive:", *DriveID)
		root, err := b.gc.Drives().ByDriveId(*DriveID).Root().Get(context.Background(), nil)
		if err != nil {
			return "", err
		}
		// Extrahieren Sie die Download-URL aus den erhaltenen Daten
		additionalData := root.GetAdditionalData()
		if url, ok := additionalData["@microsoft.graph.downloadUrl"].(string); ok {
			return url, nil
		}

	} else {
		fmt.Println("No drives found.")
	}

	return "", nil
}

// handleDownloadFile handles file download
func (b *Bmsteams) handleDownloadFile(rmsg *config.Message, filename, weburl string) error {
	realURL, err := b.findFile(weburl)
	if err != nil {
		return err
	}
	// Actually download the file.
	data, err := helper.DownloadFile(realURL)
	if err != nil {
		return fmt.Errorf("download %s failed %#v", weburl, err)
	}

	// If a comment is attached to the file(s) it is in the 'Text' field of the teams messge event
	// and should be added as comment to only one of the files. We reset the 'Text' field to ensure
	// that the comment is not duplicated.
	comment := rmsg.Text
	rmsg.Text = ""
	helper.HandleDownloadData(b.Log, rmsg, filename, comment, weburl, data, b.General)
	return nil
}

func (b *Bmsteams) handleAttachments(rmsg *config.Message, msg graphmodels.ChatMessageable) { // ->   yaegashi-Bibliothek
	for _, a := range msg.GetAttachments() {
		//remove the attachment tags from the text
		rmsg.Text = attachRE.ReplaceAllString(rmsg.Text, "")

		//handle a code snippet (code block)
		if *a.GetContentType() == "application/vnd.microsoft.card.codesnippet" {
			b.handleCodeSnippet(rmsg, a)
			continue
		}

		//handle the download
		err := b.handleDownloadFile(rmsg, *a.GetName(), *a.GetContentUrl())
		if err != nil {
			b.Log.Errorf("download of %s failed: %s", *a.GetName(), err)
		}
	}
}

type AttachContent struct {
	Language       string `json:"language"`
	CodeSnippetURL string `json:"codeSnippetUrl"`
}

// func (b *Bmsteams) handleCodeSnippet(rmsg *config.Message, attach graphmodels.ChatMessageAttachmentable) { // ->   yaegashi-Bibliothek
// 	var content AttachContent
// 	err := json.Unmarshal([]byte(*attach.GetContentType()), &content)
// 	if err != nil {
// 		b.Log.Errorf("unmarshal codesnippet failed: %s", err)
// 		return
// 	}
// 	s := strings.Split(content.CodeSnippetURL, "/")
// 	if len(s) != 13 {
// 		b.Log.Errorf("codesnippetUrl has unexpected size: %s", content.CodeSnippetURL)
// 		return
// 	}
// 	resp, err := b.gc.Teams().Request().Client().Get(content.CodeSnippetURL)
// 	if err != nil {
// 		b.Log.Errorf("retrieving snippet content failed:%s", err)
// 		return
// 	}
// 	defer resp.Body.Close()
// 	res, err := ioutil.ReadAll(resp.Body)
// 	if err != nil {
// 		b.Log.Errorf("reading snippet data failed: %s", err)
// 		return
// 	}
// 	rmsg.Text = rmsg.Text + "\n```" + content.Language + "\n" + string(res) + "\n```\n"
// }

func (b *Bmsteams) handleCodeSnippet(rmsg *config.Message, attach graphmodels.ChatMessageAttachmentable) {
	var content AttachContent
	err := json.Unmarshal([]byte(*attach.GetContentType()), &content)
	if err != nil {
		b.Log.Errorf("unmarshal codesnippet failed: %s", err)
		return
	}

	s := strings.Split(content.CodeSnippetURL, "/")
	if len(s) != 13 {
		b.Log.Errorf("codesnippetUrl has unexpected size: %s", content.CodeSnippetURL)
		return
	}

	// Definieren Sie einen neuen http.Client
	client := &http.Client{}

	// Erstellen Sie eine neue GET-Anfrage
	req, err := http.NewRequest("GET", content.CodeSnippetURL, nil)
	if err != nil {
		b.Log.Errorf("failed to create request: %s", err)
		return
	}

	// Senden Sie die Anfrage
	resp, err := client.Do(req)
	if err != nil {
		b.Log.Errorf("retrieving snippet content failed: %s", err)
		return
	}
	defer resp.Body.Close()

	res, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		b.Log.Errorf("reading snippet data failed: %s", err)
		return
	}
	rmsg.Text = rmsg.Text + "\n```" + content.Language + "\n" + string(res) + "\n```\n"
}
