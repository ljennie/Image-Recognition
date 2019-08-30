package main

import (
	elastic "gopkg.in/olivere/elastic.v3"
	"fmt"
	"net/http"
	"encoding/json"
	"log"
	"strconv"
	// "github.com/olivere/elastic"
	"github.com/pborman/uuid"
	"reflect"
	"context"
	"cloud.google.com/go/storage"
	"io"
	"github.com/auth0/go-jwt-middleware"
	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	// "cloud.google.com/go/bigtable" 
	
	"path/filepath"
)

type Location struct {//struct相当于Java的class
      Lat float64 `json:"lat"` //float64相当于Java的double //`json:"lat"` ``告诉你用的是raw string 
      Lon float64 `json:"lon"`
}

type Post struct {
      // `json:"user"` is for the json parsing of this User field. Otherwise, by default it's 'User'.
      User     string `json:"user"` //go里的User对于json里的user
      Message  string  `json:"message"`
      Location Location `json:"location"`
	  Url string `json:"url"`
	  Type     string   `json:"type"`// 告诉前端上传的type
	  Face     float64  `json:"face"`// face return score[0]
}
/* {
	"user": "john"
	"message": "Test"
	"Location": {
		"lat": 37,
		"lon": -120
	}
}
*/

//相当于java的final
const (//全大些表示一个常量
	INDEX = "around"//不同应用数据，我们现在用around
	TYPE = "post"
	DISTANCE = "200km"
	ES_URL = "http://104.197.141.2:9200"
	BUCKET_NAME = "post-images-235821"
	PROJECT_ID = "spark-235821"
	// BT_INSTANCE = "around-post"
	API_PREFIX = "/api/v1"// If deploy together with backend.
)

var mySigningKey = []byte("secret")

var (
	mediaTypes = map[string]string{ // 上传照片的mapping
	   ".jpeg": "image",
	   ".jpg":  "image",
	   ".gif":  "image",
	   ".png":  "image",
	   ".mov":  "video",
	   ".mp4":  "video",
	   ".avi":  "video",
	   ".flv":  "video",
	   ".wmv":  "video",
	}
  )
  

func main() {
	// ctx := context.Background()

	// projectID := "spark-235821"

	// Create a client
	client, err := elastic.NewClient(elastic.SetURL(ES_URL), elastic.SetSniff(false))
	if err != nil {
		panic(err)
		return
	}

	// Use the IndexExists service to check if a specified index exists.
	exists, err := client.IndexExists(INDEX).Do()
	if err != nil {
		panic(err)
	}
	if !exists {
		// Create a new index.
		mapping := `{
			"mappings":{
				"post":{
					"properties":{
						"location":{
							"type":"geo_point"
						}
					}
				}
			}
		}`
		//可以把location变成一个kd tree来管理的方式
		_, err := client.CreateIndex(INDEX).Body(mapping).Do()
		if err != nil {
			// Handle error
			panic(err)
		}
	}

	fmt.Println("started-service")

	r := mux.NewRouter()

	var jwtMiddleware = jwtmiddleware.New(jwtmiddleware.Options{
		   ValidationKeyGetter: func(token *jwt.Token) (interface{}, error) {
				  return mySigningKey, nil
		   },
		   SigningMethod: jwt.SigningMethodHS256,
	})  // 这是secret

//收到post，转到jwtMiddleware里，jwtMiddleware验证客户提交的token能不能和SigningKey信息一致，如果一致再把他转交给
//http.HandlerFunc(handlerPost)
	r.Handle(API_PREFIX+"/post", jwtMiddleware.Handler(http.HandlerFunc(handlerPost)))
	//.Methods("POST")
	r.Handle(API_PREFIX+"/search", jwtMiddleware.Handler(http.HandlerFunc(handlerSearch)))
	//.Methods("GET")
	//这时候还没有login，还不需要token验证，所以不需要用jwtMiddleware包裹
	r.Handle(API_PREFIX+"/login", http.HandlerFunc(loginHandler))
	//.Methods("POST")
	r.Handle(API_PREFIX+"/signup", http.HandlerFunc(signupHandler))
	//.Methods("POST")
	r.Handle(API_PREFIX+"/cluster", jwtMiddleware.Handler(http.HandlerFunc(handlerCluster)))
	//.Methods("GET")

	// Backend endpoints.
	http.Handle(API_PREFIX+"/", r)
	// Frontend endpoints.
	http.Handle("/", http.FileServer(http.Dir("build")))//前端数据(html css js等)都在build这个folder里面

	//http.Handle("/", r)//不加这个"/"，会返回404 router
	log.Fatal(http.ListenAndServe(":8080", nil))



	// http.HandleFunc("/post", handlerPost)//收到post直接调用handlerPost这个请求，“/post”相当于Servlet
	// http.HandleFunc("/search", handlerSearch)
	// log.Fatal(http.ListenAndServe(":8080", nil))//如果前面发生错误的话，就让他打印一个错误日志然后退出
}

// user发送的request要在http.Request里读出来
// http.ResponseWriter是server返回给浏览器的值
func handlerPost(w http.ResponseWriter, r *http.Request) {
	// for storage
	// 为了保证前端的数据，先设置这三个set
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")

	// 可以读正经的用户名了
	user := r.Context().Value("user")
    claims := user.(*jwt.Token).Claims
    username := claims.(jwt.MapClaims)["username"]


	// 32 << 20 is the maxMemory param for ParseMultipartForm, equals to 32MB (1MB = 1024 * 1024 bytes = 2^20 bytes)
	// After you call ParseMultipartForm, the file will be saved in the server memory with maxMemory size.
	// If the file size is larger than maxMemory, the rest of the data will be saved in a system temporary file.
	r.ParseMultipartForm(32 << 20)//能提交的最大form data是32兆

	// Parse from form data.
	// message
	fmt.Printf("Received one post request %s\n", r.FormValue("message"))//读出message的部分
	lat, _ := strconv.ParseFloat(r.FormValue("lat"), 64)//原来是json读取，现在读取value //:= 赋值＋定义
	lon, _ := strconv.ParseFloat(r.FormValue("lon"), 64)
	// 人工从form data里读取了数据
	p := &Post{
			User:    username.(string),
			// User:    r.FormValue.("user"),
			Message: r.FormValue("message"),
			Location: Location{
				Lat: lat,
				Lon: lon,
			},//go要求每个数据都要用，
	}

	id := uuid.New()

	

	// image
	file, _, err := r.FormFile("image")//FormFile:读取file类型的数据类型
	if err != nil {
			http.Error(w, "GCS is not setup", http.StatusInternalServerError)
			fmt.Printf("GCS is not setup %v\n", err)//%v代表后面什么类型的数据类型都可以输出
			panic(err)//Panic会返回status，告诉你哪里发生了错误
	}
	defer file.Close()

	ctx := context.Background()
	// ctx是context,存取gcs的时候需要application q...，context可以吧q 读出来才可以访问数据

	// replace it with your real bucket name.
	_, attrs, err := saveToGCS(ctx, file, BUCKET_NAME, id)//把数据存储到gcs bucket里去
	if err != nil {
			http.Error(w, "GCS is not setup", http.StatusInternalServerError)
			fmt.Printf("GCS is not setup %v\n", err)
			panic(err)//告诉我们哪行出了错
	}

	// Update the media link after saving to GCS.
	p.Url = attrs.MediaLink
	
	// Save to ES.
	saveToES(p, id)//p 变成指针

	

	//file, _, err := r.FormFile("image") 这句话虽然之前已经读过文件了，但是saveToGCS已经把文件存走了，file里已经没有文件了
	im, header, _ := r.FormFile("image")//读一下用户发来的image这个form
	defer im.Close()
	suffix := filepath.Ext(header.Filename)//读到它的文件类型

	// Client needs to know the media type so as to render it.
	if t, ok := mediaTypes[suffix]; ok {// 判断这个类型是不是在之前定义的文件里，image或者video
  		p.Type = t
	} else {
  		p.Type = "unknown"
	}
	// ML Engine only supports jpeg!!!
	// if suffix == ".jpeg" {// 如果是jpeg，那就调用annotate这个tag
  		// if score, err := annotate(im); err != nil {
     	// 	http.Error(w, "Failed to annotate the image", http.StatusInternalServerError)
     	// 	fmt.Printf("Failed to annotate the image %v\n", err)
     	// 	return
  		// } else {
     	// 	p.Face = score
		  // }
	// 	  return
	// }


	

	
	// if err != nil {
	// 	http.Error(w, "Failed to save post to ElasticSearch", http.StatusInternalServerError)
	// 	fmt.Printf("Failed to save post to ElasticSearch %v.\n", err)
	// 	return
	// }
	// fmt.Printf("Save one post to ElasticSearch: %s, p.Message")


	// Save to BigTable.//把ES存的复制到bigTable
	// saveToBigTable(p, id)
}

	  

	/*
      // Parse from body of request to get a json object.
      fmt.Println("Received one post request")
	  decoder := json.NewDecoder(r.Body)//user的request是json格式，也就是上面那组common的，然后decode
	  //把body的格式decode成Json的形式，也就是上面的raw data
      var p Post
      if err := decoder.Decode(&p); err != nil {//decoder的内容decode出来assign给了p
             panic(err)
	  }
	  //if里，分号前后是两个statment，第一个statment做初始化的变量，第二个statment做变量的判断
	  //&p:p的指针(address)传入到了Decode作为参数,这样Decode才能修改p里面的值，这时候p的值改变了，
      //也就变成了用户提交的json，就变成用户post的值,改变了外面的值
	  fmt.Fprintf(w, "Post received: %s\n", p.Message)
	  //"Post received: %s\n", p.Message 给 w
	  //printf是自动往console里面输入 Fprintf的第一个F 是file，可以指定往哪里输出

	  id := uuid.New()//uuid: create a unique id来区分和别的不一样
      // Save to ES.
      saveToES(&p, id)
*/

func saveToGCS(ctx context.Context, r io.Reader, bucketName, name string)(*storage.ObjectHandle, *storage.ObjectAttrs, error) {
	// create client
	client, err := storage.NewClient(ctx)
      if err != nil {
             return nil, nil, err
      }
      defer client.Close()
	// bucket has been created, we need to check if exist
	// create a bucket instance
	bucket := client.Bucket(bucketName)//BucketName创建的bucket，google storage通过client来访问这个bucket
	//check if exist
	if _, err = bucket.Attrs(ctx); err != nil {
		return nil, nil, err
 	}

	obj := bucket.Object(name)
	// write client
	wc := obj.NewWriter(ctx)
	if _, err := io.Copy(wc, r); err != nil {
		return nil, nil, err
 	}
	if err := wc.Close(); err != nil {
		return nil, nil, err
 	}

	//到这里没有人有权限可以去读
	//ACL: access control list
	if err := obj.ACL().Set(ctx, storage.AllUsers, storage.RoleReader); err != nil {//这里标记所有用户可以去读
		return nil, nil, err
	}
	// 这时候可以访问文件
	// 文件放在了gcs 上了，我们需要获取文件的url
	attrs, err := obj.Attrs(ctx)
    fmt.Printf("Post is saved to GCS: %s\n", attrs.MediaLink)
    return obj, attrs, err
}
// func saveToBigTable(p *Post, id string) {
// 	ctx := context.Background()// ctx可以存储除了核心东西以外的其他东西
// 	// you must update project name here
// 	// create client, create connection to bigTable
// 	bt_client, err := bigtable.NewClient(ctx, PROJECT_ID, BT_INSTANCE)
// 	if err != nil {
// 		   panic(err)
// 		   return
// 	}

// 	tbl := bt_client.Open("post")//open刚create好的table
// 	mut := bigtable.NewMutation()//想要对big table 操作的一个行 one row
// 	t := bigtable.Now() 

// 	mut.Set("post", "user", t, []byte(p.User))
// 	mut.Set("post", "message", t, []byte(p.Message))
// 	mut.Set("location", "lat", t, []byte(strconv.FormatFloat(p.Location.Lat, 'f', -1, 64)))
// 	mut.Set("location", "lon", t, []byte(strconv.FormatFloat(p.Location.Lon, 'f', -1, 64)))
// 	// p is time stamp

// 	err = tbl.Apply(ctx, id, mut)
// 	if err != nil {
// 		   panic(err)
// 		   return
// 	}
// 	fmt.Printf("Post is saved to BigTable: %s\n", p.Message)

// }


// Save a post to ES
func saveToES(p *Post, id string) {
	fmt.Printf("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	// Create a client
	es_client, err := elastic.NewClient(elastic.SetURL(ES_URL), elastic.SetSniff(false))
	if err != nil {
		panic(err)
		return
	}

	// Save it to index
	_, err = es_client.Index().
		Index(INDEX).
		Type(TYPE).//post
		Id(id).
		BodyJson(p).
		Refresh(true).//如果我们有新的ID，重复旧的
		Do()
	if err != nil {
		panic(err)
		return
	}

	fmt.Printf("Post is saved to Index: %s\n", p.Message)
}


//通过Current location发送给后端，后端找到对应的数据，然后找到post类型的数据
// 返回搜索附近别人发的post
func handlerSearch(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Received one request for search")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
	// w.Header().Set("Access-Control-Allow-Header", "Content-Type")
	// w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	// w.Write(js)

	lat, _ := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)//64 相当于double in java
    lon, _ := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	// range is optional 
	ran := DISTANCE 
	val := r.URL.Query().Get("range")
	if val != "" { 
   		ran = val + "km"
	}
	fmt.Printf( "Search received: %f %f %s\n", lat, lon, ran)

    // Create a client
	client, err := elastic.NewClient(elastic.SetURL(ES_URL), elastic.SetSniff(false))
	//SetSniff:ES有个功能，每次操作有个callback，然后有个log；例如：有多少人通过我的程序来访问这个系统,callback
	if err != nil {
			panic(err)
	}

	// Define geo distance query as specified in
	// https://www.elastic.co/guide/en/elasticsearch/reference/5.2/query-dsl-geo-distance-query.html
	q := elastic.NewGeoDistanceQuery("location")
	q = q.Distance(ran).Lat(lat).Lon(lon)

	// Some delay may range from seconds to minutes. So if you don't get enough results. Try it later.
	searchResult, err := client.Search().
			Index(INDEX).
			Query(q).
			Pretty(true).//为了返回的json好看一点
			Do()//最后一步才进行搜索
	if err != nil {
			// Handle error
			panic(err)
	}

	// searchResult is of type SearchResult and returns hits, suggestions,
	// and all kinds of other information from Elasticsearch.
	fmt.Printf("Query took %d milliseconds\n", searchResult.TookInMillis)
	// TotalHits is another convenience function that works even when something goes wrong.
	fmt.Printf("Found a total of %d post\n", searchResult.TotalHits())

	// Each is a convenience function that iterates over hits in a search result.
	// It makes sure you don't need to check for nil values in the response.
	// However, it ignores errors in serialization.
	var typ Post
	var ps []Post
	for _, item := range searchResult.Each(reflect.TypeOf(typ)) { 
			p := item.(Post) // p = (Post) item
			fmt.Printf("Post by %s: %s at lat %v and lon %v\n", p.User, p.Message, p.Location.Lat, p.Location.Lon)
			// TODO(student homework): Perform filtering based on keywords such as web spam etc.
			ps = append(ps, p)

	}
	js, err := json.Marshal(ps)
	if err != nil {
			panic(err)
			return
	}
	w.Write(js)

	

	



// fmt.Println("range is ", ran)
// // Return a fake post
// p := &Post{
// 	User:"1111",
// 	Message:"一生必去的100个地方",
// 	Location: Location{
// 		   Lat:lat,
// 		   Lon:lon,
// 	},
//  }
	
//  js, err := json.Marshal(p)//go的数据结构变成json string
//  if err != nil {
//          panic(err)
//  }
	
//  w.Header().Set("Content-Type", "application/json")
//  w.Write(js)

// fmt.Fprintf(w, "Search received: %f %f", lat, lon)
}


func handlerCluster(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Received one request for clustering")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
	// 方便前端老师用

	if r.Method != "GET" {
		return
	}

	term := r.URL.Query().Get("term")

	// Create a client
	client, err := elastic.NewClient(elastic.SetURL(ES_URL), elastic.SetSniff(false))
	if err != nil {
		http.Error(w, "ES is not setup", http.StatusInternalServerError)
		fmt.Printf("ES is not setup %v\n", err)
		return
	}

	// Range query.
	// For details, https://www.elastic.co/guide/en/elasticsearch/reference/current/query-dsl-range-query.html
	q := elastic.NewRangeQuery(term).Gte(0.9)

	searchResult, err := client.Search().
		Index(INDEX).
		Query(q).
		Pretty(true).
		Do()
	if err != nil {
		// Handle error
		m := fmt.Sprintf("Failed to query ES %v", err)
		fmt.Println(m)
		http.Error(w, m, http.StatusInternalServerError)
	}

	// searchResult is of type SearchResult and returns hits, suggestions,
	// and all kinds of other information from Elasticsearch.
	fmt.Printf("Query took %d milliseconds\n", searchResult.TookInMillis)
	// TotalHits is another convenience function that works even when something goes wrong.
	fmt.Printf("Found a total of %d post\n", searchResult.TotalHits())

	// Each is a convenience function that iterates over hits in a search result.
	// It makes sure you don't need to check for nil values in the response.
	// However, it ignores errors in serialization.
	var typ Post
	var ps []Post
	for _, item := range searchResult.Each(reflect.TypeOf(typ)) {
		p := item.(Post)
		ps = append(ps, p)

	}
	js, err := json.Marshal(ps)
	if err != nil {
		m := fmt.Sprintf("Failed to parse post object %v", err)
		fmt.Println(m)
		http.Error(w, m, http.StatusInternalServerError)
		return
	}

	w.Write(js)
}


/*
func handlerCluster(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Received one request for search")
	

      // Create a client
	client, err := elastic.NewClient(elastic.SetURL(ES_URL), elastic.SetSniff(false))
	//SetSniff:有多少人通过我的程序来访问这个系统,callback
	if err != nil {
			panic(err)
	}

	// Define geo distance query as specified in
	// https://www.elastic.co/guide/en/elasticsearch/reference/5.2/query-dsl-geo-distance-query.html
	q := elastic.NewRangeQuery("face").Gte(0.8)
	//Gte: greater than equal
	

	// Some delay may range from seconds to minutes. So if you don't get enough results. Try it later.
	searchResult, err := client.Search().
			Index(INDEX).
			Query(q).
			Pretty(true).
			Do()
	if err != nil {
			// Handle error
			panic(err)
	}

	// searchResult is of type SearchResult and returns hits, suggestions,
	// and all kinds of other information from Elasticsearch.
	fmt.Printf("Query took %d milliseconds\n", searchResult.TookInMillis)
	// TotalHits is another convenience function that works even when something goes wrong.
	fmt.Printf("Found a total of %d post\n", searchResult.TotalHits())

	// Each is a convenience function that iterates over hits in a search result.
	// It makes sure you don't need to check for nil values in the response.
	// However, it ignores errors in serialization.
	var typ Post
	var ps []Post
	for _, item := range searchResult.Each(reflect.TypeOf(typ)) { 
			p := item.(Post) // p = (Post) item
			fmt.Printf("Post by %s: %s at lat %v and lon %v\n", p.User, p.Message, p.Location.Lat, p.Location.Lon)
			// TODO(student homework): Perform filtering based on keywords such as web spam etc.
			ps = append(ps, p)

	}
	js, err := json.Marshal(ps)
	if err != nil {
			panic(err)
			return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(js)
}
*/

