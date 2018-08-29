package main

import (
  "context"
  "encoding/json"
  "fmt"
  "github.com/pkg/errors"
  "golang.org/x/oauth2/google"
  "io"
  "io/ioutil"
  "net/http"
  "strings"
)

type Prediction struct {// java class
  Prediction int       `json:"prediction"`
  Key        string    `json:"key"`
  Scores     []float64 `json:"scores"`
}

type MlResponse struct {// 把前面的Prediction包裹起来
  Predictions []Prediction `json:"predictions"`
}
// {
// 	"predictions": [
// 	  {
// 		"label": "beach",
// 		"scores": [0.1, 0.9]
// 	  },
// 	  {
// 		"label": "car",
// 		"scores": [0.75, 0.25]
// 	  }
// 	]
//   }

type ImageBytes struct {
  B64 []byte `json:"b64"`
}

type Instance struct {
  ImageBytes ImageBytes `json:"image_bytes"`
  Key        string     `json:"key"`
}
// {
// 	"instances": [
// 	  {
// 		"tag": "beach",
// 		"image": {"b64": "ASa8asdf"}
// 	  },
// 	  {
// 		"tag": "car",
// 		"image": {"b64": "JLK7ljk3"}
// 	  }
// 	]
//   }


type MlRequest struct {//每一个单独的object
  Instances []Instance `json:"instances"`
}

var (
  project = "united-strategy-206620"
  model = "face"
  url   = "https://ml.googleapis.com/v1/projects/" + project + "/models/" + model + ":predict"
  scope = "https://www.googleapis.com/auth/cloud-platform"// 我们想访问cloud这个scope
)
  

// Annotate a image file based on ml model, return score and error if exists.
func annotate(r io.Reader) (float64, error) {// label image的结果， r io.Reader输入的file string， float64：返回 image 是face的可能性
	ctx := context.Background()
	buf, _ := ioutil.ReadAll(r)// 从io.reader 读取buffle
  
	ts, err := google.DefaultTokenSource(ctx, scope) // 问Google要token
	if err != nil {
	   fmt.Printf("failed to create token %v\n", err)
	   return 0.0, err
	}
	tt, _ := ts.Token()
  
	// Construct a ml request.
	request := &MlRequest{
	   Instances: []Instance{
		  {
			 ImageBytes: ImageBytes{
				B64: buf,
			 },
			 Key: "1", // Does not matter to the client, it's for Google tracking.
		  },
	   },
	}
	body, _ := json.Marshal(request)//把整个request转成json格式的body
	// Construct a http request.
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+tt.AccessToken)//把token加进去
  
	fmt.Printf("Sending request to ml engine for prediction %s with token as %s\n", url, tt.AccessToken)
	// Send request to Google.
	client := &http.Client{}
	res, err := client.Do(req)// 发送request，execute
	if err != nil {
	   fmt.Printf("failed to send ml request %v\n", err)
	   return 0.0, err
	}
	var resp MlResponse
	body, _ = ioutil.ReadAll(res.Body)// 把response拿来作为解析
  
	// Double check if the response is empty. Sometimes Google does not return an error instead just an
	// empty response while usually it's due to auth.
	if len(body) == 0 {// 判断body是不是有数据
	   fmt.Println("empty google response")
	   return 0.0, errors.New("empty google response")
	}
	if err := json.Unmarshal(body, &resp); err != nil {//Unmarshal：把go里面的structure变成一个string
	   fmt.Printf("failed to parse response %v\n", err)
	   return 0.0, err
	}
  
	if len(resp.Predictions) == 0 {
	   // If the response is not empty, Google returns a different format. Check the raw message.
	   // Sometimes it's due to the image format. Google only accepts jpeg don't send png or others.
	   fmt.Printf("failed to parse response %s\n", string(body))
	   return 0.0, errors.Errorf("cannot parse response %s\n", string(body))
	}
	// TODO: update index based on your ml model.
	results := resp.Predictions[0]//Predictions[0] 第一个model
	fmt.Printf("Received a prediction result %f\n", results.Scores[0])//results.Scores[0] 第一个face的概率
	return results.Scores[0], nil
  } // high precision, low recall
  
