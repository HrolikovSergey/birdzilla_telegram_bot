package main

import (
    "fmt"
    "net/http"
    "encoding/json"
    "io/ioutil"
    "bytes"
    "reflect"
    "strings"
    "regexp"
    "os"
    "io"
    "gopkg.in/telegram-bot-api.v4"
    "github.com/arbovm/levenshtein"
)

type (

    Config struct {
        BotId      string
        AudioPath  string
        ImagesPath string
    }

    Bird struct {
        id          string
        name        string
        url         string
        description string
        songFile    string
        imageFile   string
        pageContent string
    }

    Birds []Bird
)

var (
    siteUrl = "http://www.birdzilla.com/"
    listUrl = "birds/names-aliases-json.html?output_format=json"
    dataUrl = "components/com_birds/files/%s/mp3/"
    birds = Birds{}
    conf = Config{}
)

func concat(str... string) string {
    var buff bytes.Buffer
    for _, part := range (str) {
        buff.WriteString(part)
    }
    return buff.String()
}

func (bird *Bird) mp3Name(name string) []string {
    var names []string
    if name == "" {
        name = bird.name
    }
    re, _ := regexp.Compile("['\\s]")
    names = append(names, concat(re.ReplaceAllString(name, "-"), ".mp3"))
    re, _ = regexp.Compile("[\\s]")
    names = append(names, concat(re.ReplaceAllString(strings.Replace(name, "'", "", -1), "-"), ".mp3"))
    return names
}

func (bird *Bird) sysName(name string) string {
    if name == "" {
        name = bird.name
    }
    re, _ := regexp.Compile("[-'\\s]")
    name = re.ReplaceAllString(strings.ToLower(name), "")
    return name
}

func (birds *Birds) UnmarshalJSON(b []byte) error {
    var list interface{}
    json.Unmarshal(b, &list)
    m := list.([]interface{})
    for i := range m {
        switch reflect.TypeOf(m[i]).Kind(){
        case reflect.Slice:
            s := reflect.ValueOf(m[i])
            *birds = append(*birds, Bird{
                s.Index(0).Interface().(string),
                s.Index(1).Interface().(string),
                s.Index(2).Interface().(string),
                "", "", "", ""})
        }
    }
    return nil
}

func getList() Birds {
    birds := Birds{}
    resp, err := http.Get(concat(siteUrl, listUrl))
    if err != nil {
        panic("Connection error: Load list")
    }
    defer resp.Body.Close()
    body, _ := ioutil.ReadAll(resp.Body)
    json.Unmarshal(body, &birds)
    return birds
}

func (bird *Bird) GetSong() bool {
    if bird.songFile != "" {
        return true
    } else {
        path := concat(conf.AudioPath, bird.sysName(""), ".mp3")
        if _, err := os.Stat(path); err == nil {
            bird.songFile = path
            return true
        } else {
            names := bird.mp3Name("")
            for _, name := range (names) {
                url := fmt.Sprintf(concat(siteUrl, dataUrl, name), bird.id)
                resp, err := http.Get(url)
                if err != nil {
                    fmt.Printf("Error: %s\n", err)
                    return false
                }
                defer resp.Body.Close()
                if resp.Header.Get("Content-Type") == "audio/mpeg" {
                    out, err := os.Create(path)
                    if err != nil {
                        fmt.Printf("Error: %s\n", err)
                        return false
                    }
                    _, err = io.Copy(out, resp.Body)
                    if err != nil {
                        fmt.Printf("Error: %s\n", err)
                        return false
                    }
                    out.Close()
                    bird.songFile = path
                    return true
                } else {
                    resp.Body.Close()
                }
            }
        }
    }
    return false
}

func (bird *Bird) loadPageContent() {
    resp, _ := http.Get(concat(siteUrl, bird.url))
    defer resp.Body.Close()
    body, _ := ioutil.ReadAll(resp.Body)
    bird.pageContent = string(body)
}

func (bird *Bird) getDescription() bool {
    if bird.description == "" {
        if bird.pageContent == "" {
            bird.loadPageContent()
        }
        r, err := regexp.Compile("description page-item\">((.|\\s)*?)<\\/div>")
        if err != nil {
            fmt.Println(err)
        }
        data := r.FindAllStringSubmatch(bird.pageContent, -1)
        if data != nil {
            r, err = regexp.Compile("<[^>]*>")
            if err != nil {
                fmt.Println(err)
                return false
            }
            bird.description = r.ReplaceAllString(data[0][1], "")
            return true
        } else {
            return false
        }
    }
    return true
}

func (bird *Bird) getImage() bool {
    if bird.imageFile != "" {
        return true
    } else {
        path := concat(conf.ImagesPath, bird.sysName(""), ".jpg")
        if _, err := os.Stat(path); err == nil {
            bird.imageFile = path
            return true
        } else {
            if bird.pageContent == "" {
                bird.loadPageContent()
            }
            r, err := regexp.Compile("class=\"images\">\\s+.+src=\"([^\"*]+)")
            if err != nil {
                fmt.Println(err)
            }
            data := r.FindAllStringSubmatch(bird.pageContent, -1)
            if data != nil {
                resp, err := http.Get(data[0][1])
                if err != nil {
                    fmt.Println(err)
                    return false
                }
                defer resp.Body.Close()
                if resp.Header.Get("Content-Type") == "image/jpeg" {
                    out, err := os.Create(path)
                    if err != nil {
                        fmt.Printf("Error: %s\n", err)
                        return false
                    }
                    _, err = io.Copy(out, resp.Body)
                    if err != nil {
                        fmt.Printf("Error: %s\n", err)
                        return false
                    }
                    out.Close()
                    bird.imageFile = path
                    return true
                }
            }
        }
    }
    return false
}

func (birds *Birds) getBird(request string) (Bird, bool) {
    if request != "" {
        for _, bird := range *birds {
            if bird.sysName("") == bird.sysName(request) {
                return bird, true
            }
        }
        minDistance := 999
        found := Bird{}
        for _, bird := range *birds {
            if minDistance > levenshtein.Distance(bird.sysName(""), bird.sysName(request)){
                found = bird
                minDistance = levenshtein.Distance(bird.sysName(""), bird.sysName(request))
            }
        }
        return found, true
    }
    return Bird{}, false
}

func init(){
    file, err := os.Open("conf.json")
    if (err != nil) {
        panic(err)
    }
    defer file.Close()
    decoder := json.NewDecoder(file)
    err = decoder.Decode(&conf)
    if err != nil {
        panic(err)
    }
}

func main() {
    birds = getList()
    bot, err := tgbotapi.NewBotAPI(conf.BotId)
    if err != nil {
        panic(err)
    }
    fmt.Println("Authorized on account", bot.Self.UserName)
    u := tgbotapi.NewUpdate(0)
    u.Timeout = 60
    updates, _ := bot.GetUpdatesChan(u)
    for update := range updates {
        bird, found := birds.getBird(update.Message.Text)
        fmt.Println("Request:", update.Message.Text)
        if found {
            fmt.Println("Found:", bird.name, bird.id)
            if bird.getImage() {
                msg := tgbotapi.NewPhotoUpload(update.Message.Chat.ID, bird.imageFile)
                bot.Send(msg)
            }
            if bird.getDescription() {
                msg := tgbotapi.NewMessage(update.Message.Chat.ID, bird.description)
                bot.Send(msg)
            }

            if bird.GetSong() {
                msg := tgbotapi.NewVoiceUpload(update.Message.Chat.ID, bird.songFile)
                bot.Send(msg)
            }
        } else {
            msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Bird not found.")
            bot.Send(msg)
        }
    }
}