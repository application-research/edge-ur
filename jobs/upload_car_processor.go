package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/application-research/edge-ur/utils"
	"github.com/ipfs/go-cid"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/application-research/edge-ur/core"
)

type UploadCarToDeltaProcessor struct {
	CarBucket core.CarBucket `json:"car_bucket"`
	File      io.Reader      `json:"file"`
	RootCid   string         `json:"root_cid"`
	Processor
}

func NewUploadCarToDeltaProcessor(ln *core.LightNode, bucket core.CarBucket, fileNode io.Reader, rootCid string) IProcessor {
	DELTA_UPLOAD_API = ln.Config.ExternalApi.ApiUrl
	REPLICATION_FACTOR = string(ln.Config.Common.ReplicationFactor)
	return &UploadCarToDeltaProcessor{
		bucket,
		fileNode,
		rootCid,
		Processor{
			LightNode: ln,
		},
	}
}

func (r *UploadCarToDeltaProcessor) Info() error {
	panic("implement me")
}

func (r *UploadCarToDeltaProcessor) Run() error {

	// if network connection is not available or delta node is not available, then we need to skip and
	// let the upload retry consolidate the content until it is available

	maxRetries := 5
	retryInterval := 5 * time.Second
	//var content []core.Content
	//r.LightNode.DB.Model(&core.CarBucket{}).Where("id = ?", r.CarBucket.ID).Find(&content)

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	//carCid, err := cid.Decode(r.CarBucket.Cid)
	//if err != nil {
	//	fmt.Println("Error decoding car cid: ", err)
	//	return nil
	//}
	//
	//carNode, err := r.LightNode.Node.Get(context.Background(), carCid)
	//if err != nil {
	//	fmt.Println("Error getting car node: ", err)
	//	return nil
	//}

	partFile, err := writer.CreateFormFile("data", r.CarBucket.Cid)
	if err != nil {
		fmt.Println("CreateFormFile error: ", err)
		return nil
	}
	cidToGet, err := cid.Decode(r.CarBucket.Cid)
	if err != nil {
		fmt.Println("Error decoding cid: ", err)
		return nil
	}
	rootNd, err := r.LightNode.Node.DAGService.Get(context.Background(), cidToGet)
	if err != nil {
		fmt.Println("Error getting root node: ", err)
		return nil
	}

	_, err = io.Copy(partFile, bytes.NewReader(rootNd.RawData()))
	if err != nil {
		fmt.Println("Copy error: ", err)
		return nil
	}
	if partFile, err = writer.CreateFormField("metadata"); err != nil {
		fmt.Println("CreateFormField error: ", err)
		return nil
	}
	repFactor := r.LightNode.Config.Common.ReplicationFactor
	partMetadata := fmt.Sprintf(`{"auto_retry":true,"miner":"%s","replication":%d}`, r.CarBucket.Miner, repFactor)

	fmt.Println("partMetadata: ", partMetadata)

	if _, err = partFile.Write([]byte(partMetadata)); err != nil {
		fmt.Println("Write error: ", err)
		return nil
	}
	if err = writer.Close(); err != nil {
		fmt.Println("Close error: ", err)
		return nil
	}

	req, err := http.NewRequest("POST",
		DELTA_UPLOAD_API+"/api/v1/deal/end-to-end",
		payload)

	if err != nil {
		fmt.Println(err)
		return nil
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+r.CarBucket.RequestingApiKey)
	client := &http.Client{}
	var res *http.Response
	for j := 0; j < maxRetries; j++ {
		res, err = client.Do(req)
		if err != nil || res.StatusCode != http.StatusOK {
			fmt.Printf("Error sending request (attempt %d): %v\n", j+1, err)
			time.Sleep(retryInterval)
			continue
		} else {
			if res.StatusCode == 200 {
				var dealE2EUploadResponse DealE2EUploadResponse
				body, err := ioutil.ReadAll(res.Body)
				if err != nil {
					fmt.Println(err)
					continue
				}
				err = json.Unmarshal(body, &dealE2EUploadResponse)
				if err != nil {
					fmt.Println(err)
					continue
				} else {
					if dealE2EUploadResponse.ContentID == 0 {
						continue
					} else {
						r.CarBucket.UpdatedAt = time.Now()
						r.CarBucket.Status = utils.STATUS_UPLOADED_TO_DELTA
						r.CarBucket.DeltaContentId = int64(dealE2EUploadResponse.ContentID)
						r.LightNode.DB.Save(&r.CarBucket)

						// insert each replicated content into the database
						for _, replicatedContent := range dealE2EUploadResponse.ReplicatedContents {
							var replicatedContentModel core.Content
							//r.LightNode.DB.Model(&core.Content{}).Where("cid = ?", replicatedContent.Cid).Find(&replicatedContentModel)
							//if replicatedContentModel.ID == 0 {
							replicatedContentModel.Name = r.CarBucket.Name
							replicatedContentModel.Cid = r.CarBucket.Cid
							replicatedContentModel.Size = r.CarBucket.Size
							replicatedContentModel.Status = replicatedContent.Status
							replicatedContentModel.LastMessage = replicatedContent.Message
							replicatedContentModel.DeltaNodeUrl = DELTA_UPLOAD_API
							replicatedContentModel.CreatedAt = time.Now()
							replicatedContentModel.UpdatedAt = time.Now()
							replicatedContentModel.RequestingApiKey = r.CarBucket.RequestingApiKey
							replicatedContentModel.DeltaContentId = int64(replicatedContent.ContentID)
							r.LightNode.DB.Save(&replicatedContentModel)
							//}
						}
						break
					}
				}
			}
		}
	}

	return nil
}