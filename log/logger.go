package log

type Logger interface {
	Debug(message string)
	DebugF(format string, args ...interface{})
	DebugE(err error)
	DebugD(data interface{})
	Info(message string)
	InfoE(err error)
	InfoD(data interface{})
	InfoF(format string, args ...interface{})
	Notice(message string)
	NoticeE(err error)
	NoticeD(data interface{})
	NoticeF(format string, args ...interface{})
	Warning(message string)
	WarningE(err error)
	WarningD(data interface{})
	WarningF(format string, args ...interface{})
	Error(message string)
	ErrorE(err error)
	ErrorD(data interface{})
	ErrorF(format string, args ...interface{})
	Critical(message string)
	CriticalE(err error)
	CriticalD(data interface{})
	CriticalF(format string, args ...interface{})
	Alert(message string)
	AlertE(err error)
	AlertD(data interface{})
	AlertF(format string, args ...interface{})
	Emergency(message string)
	EmergencyE(err error)
	EmergencyD(data interface{})
	EmergencyF(format string, args ...interface{})
}

