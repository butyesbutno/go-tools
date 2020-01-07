package commonLog

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

//LogUtil log util
type LogUtil struct {
	Logger *log.Logger
	Level  int
	Enable bool
	File   *os.File
	Time   time.Time
	Locker *sync.Mutex
}

const (
	//CRITILAL critial message. program not able to run
	CRITILAL = 0
	//ERROR serial message, cause function fail
	ERROR = 1
	//WARN warning message
	WARN = 2
	//INFO information messag
	INFO = 3
)

var (
	localLogutil *LogUtil
	LogPrefix = "lock-"
	BaseLogPath = "./log/"
)

func init() {
	fileName := getLogFileName(time.Now())
	logFile, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Failed to open file in log.init. %s", err.Error())
		//panic(fileName)
	}

	write := io.MultiWriter(logFile, os.Stdout)
	gin.DefaultWriter = write
	gin.DefaultErrorWriter = write
	localLogutil = &LogUtil{
		Logger: log.New(write, "",
			log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile),
		Level:  INFO,
		Enable: true,
		File:   logFile,
		Time:   time.Now(),
		Locker: new(sync.Mutex)}
}

func getLogFileName(t time.Time) string {
	day := t.Format("20060102")
	curDir := getBaseDiskPath()

	//curDir, _ := os.Getwd()
	logDir := path.Join(curDir, "log")

	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		if er := os.MkdirAll(logDir, 0666); er != nil {
			panic(fmt.Sprintf("cant't create %s. error : %s",
				logDir, er.Error()))
		}
	}

	fileName := path.Join(logDir, LogPrefix+day+".log")
	return fileName
}

func rotateLog() {
	tNow := time.Now()
	if tNow.Day() == localLogutil.Time.Day() {
		return
	}
	localLogutil.Locker.Lock()
	defer localLogutil.Locker.Unlock()
	if tNow.Day() == localLogutil.Time.Day() {
		return
	}
	localLogutil.File.Close()
	localLogutil.Time = tNow

	fileName := getLogFileName(tNow)

	logFile, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Failed to open file in log.init. %s", err.Error())
		panic(fileName)
	}
	localLogutil.File = logFile
	write := io.MultiWriter(logFile, os.Stdout)
	gin.DefaultWriter = write
	gin.DefaultErrorWriter = write

	localLogutil.Logger = log.New(write, "",
		log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
}

//LogSetoutput set output
func LogSetoutput(w io.Writer) {
	localLogutil.Logger.SetOutput(w)
}

//GetLogWriter return io.Writer for the log file
func GetLogWriter() *os.File {
	return localLogutil.File
}

//LogEnable enable and disable the log output
func LogEnable(val bool) {
	localLogutil.Enable = val
}

//LogIsEnabled judge whether the log enable or not
func LogIsEnabled() bool {
	return localLogutil.Enable
}

//LogLevel set the log level
func LogLevel(level int) {
	localLogutil.Level = level
}

func baseLog(level, format string, v ...interface{}) {
	str := fmt.Sprintf(format, v...)
	localLogutil.Logger.SetPrefix(level)
	localLogutil.Logger.Output(3, str)
	rotateLog()
}

//LogCritial log critial level log
func LogCritial(format string, v ...interface{}) {
	if localLogutil.Enable == false {
		return
	}
	if localLogutil.Level >= CRITILAL {
		baseLog("CRITILAL ", format, v...)
	}
}

//LogError log error level message
func LogError(format string, v ...interface{}) {
	if localLogutil.Enable == false {
		return
	}
	if localLogutil.Level >= ERROR {
		baseLog("ERROR ", format, v...)
	}
}

//LogWarn log error level message
func LogWarn(format string, v ...interface{}) {
	if localLogutil.Enable == false {
		return
	}
	if localLogutil.Level >= WARN {
		baseLog("WARN ", format, v...)
	}
}

//LogInfo log error level message
func LogInfo(format string, v ...interface{}) {
	if localLogutil.Enable == false {
		return
	}
	if localLogutil.Level >= INFO {
		baseLog("INFO ", format, v...)
	}
}

// getBaseDiskPath go-web server base path
func getBaseDiskPath() string {
	curDir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	curDir = filepath.Join(curDir, BaseLogPath)
	return curDir
}