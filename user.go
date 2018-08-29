package main

import (
	elastic "gopkg.in/olivere/elastic.v3"

	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"time"

	"github.com/dgrijalva/jwt-go"
)

const (
	TYPE_USER = "user"//"user"是往elasticSearch保存用户信息的时候需要使用的
)

var (
	usernamePattern = regexp.MustCompile(`^[a-z0-9_]+$`).MatchString
	//regular expression 正则表达式：是用来判断username是否规范（小写字母 或者 数字 或者下划线）
	// ^ 匹配到字符串的一开始   $ 匹配到字符串末尾  ［ ］表示里面可以使用的字符号 ＋表示匹配一个或更多
	// *表示匹配零个或更多
)

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Age int `json:"age"`
	Gender string `json:"gender"`
}


// checkUser checks whether user is valid
func checkUser(username, password string) bool {//数据还是存在elastic search里的
	es_client, err := elastic.NewClient(elastic.SetURL(ES_URL), elastic.SetSniff(false))
	if err != nil {
		   fmt.Printf("ES is not setup %v\n", err)
		   panic(err)
	}

	// Search with a term query
	// termQuery查keyword
	termQuery := elastic.NewTermQuery("username", username)
	queryResult, err := es_client.Search().
		   Index(INDEX).// elastic seach 里的index
		   Query(termQuery).
		   Pretty(true).
		   Do()
	if err != nil {
		   fmt.Printf("ES query failed %v\n", err)
		   panic(err)
	}

	var tyu User
	// reflection 从搜索结果中找出用户的数据
	for _, item := range queryResult.Each(reflect.TypeOf(tyu)) {
		   u := item.(User)
		   return u.Password == password && u.Username == username
	}
	// If no user exist, return false.
	return false
}


// Add a new user. Return true if successfully.
func addUser(user User) bool {
	es_client, err := elastic.NewClient(elastic.SetURL(ES_URL), elastic.SetSniff(false))
	if err != nil {
		fmt.Printf("ES is not setup %v\n", err)
		return false
	}

	//检查用户是否存在
	termQuery := elastic.NewTermQuery("username", user.Username)
	queryResult, err := es_client.Search().
		Index(INDEX).
		Query(termQuery).
		Pretty(true).
		Do()
	if err != nil {
		fmt.Printf("ES query failed %v\n", err)
		return false
	}

	if queryResult.TotalHits() > 0 {
		fmt.Printf("User %s already exists, cannot create duplicate user.\n", user.Username)
		return false
	}

	_, err = es_client.Index().
		Index(INDEX).
		Type(TYPE_USER).
		Id(user.Username).
		BodyJson(user).
		Refresh(true).
		Do()
	if err != nil {
		fmt.Printf("ES save user failed %v\n", err)
		return false
	}

	return true
}



// If signup is successful, a new session is created.
func signupHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Received one signup request")

	decoder := json.NewDecoder(r.Body)
	var u User
	if err := decoder.Decode(&u); err != nil {
		   panic(err)
		   return
	}

	//用户名密码是否符合规范，yes的话就创建user
	if u.Username != "" && u.Password != "" && usernamePattern(u.Username) {
		   if addUser(u) {
				  fmt.Println("User added successfully.")
				  w.Write([]byte("User added successfully."))//w.Write的内容会返回给客户的信息
		   } else {
				  fmt.Println("Failed to add a new user.")
				  http.Error(w, "Failed to add a new user", http.StatusInternalServerError)
				  //http.Error通过err的形式来通知客户
		   }
	} else {
		   fmt.Println("Empty password or username.")
		   http.Error(w, "Empty password or username", http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "text/plain")//返回结果是文本
	w.Header().Set("Access-Control-Allow-Origin", "*")//所有人都可以访问
}



// If login is successful, a new token is created.
func loginHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Received one login request")

	decoder := json.NewDecoder(r.Body)
	var u User
	if err := decoder.Decode(&u); err != nil {
		   panic(err)
		   return
	}

	if checkUser(u.Username, u.Password) {// 用户名密码是否匹配成功
		   token := jwt.New(jwt.SigningMethodHS256)//生成token 返回给用户
		   claims := token.Claims.(jwt.MapClaims)// payload
		   /* Set token claims */
		   claims["username"] = u.Username
		   claims["exp"] = time.Now().Add(time.Hour * 24).Unix()//Unix时间，1970／01/01开始的秒数

		   /* Sign the token with our secret */
		   tokenString, _ := token.SignedString(mySigningKey)// verify signature

		   /* Finally, write the token to the browser window */
		   w.Write([]byte(tokenString))
	} else {
		   fmt.Println("Invalid password or username.")
		   http.Error(w, "Invalid password or username", http.StatusForbidden)
		   // 不符合用户名密码格式就会返回forbidden
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Access-Control-Allow-Origin", "*")
}


