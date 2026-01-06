package shell

const scrollbackMaxLines = 200

var builtinCommands = []string{
	"cat",
	"cd",
	"clear",
	"cp",
	"echo",
	"help",
	"ls",
	"log",
	"mkdir",
	"panic",
	"put",
	"pwd",
	"rtdemo",
	"scrollback",
	"stat",
	"ticks",
	"touch",
	"uname",
	"uptime",
	"vi",
	"version",
}

type commandHelp struct {
	Name string
	Desc string
}

var builtinCommandHelp = []commandHelp{
	{Name: "help", Desc: "Show available commands."},
	{Name: "clear", Desc: "Clear the terminal."},
	{Name: "echo", Desc: "Print arguments."},
	{Name: "cat", Desc: "Print a file."},
	{Name: "ls", Desc: "List directory entries."},
	{Name: "pwd", Desc: "Print current directory."},
	{Name: "cd", Desc: "Change current directory."},
	{Name: "mkdir", Desc: "Create a directory."},
	{Name: "touch", Desc: "Create file if missing."},
	{Name: "cp", Desc: "Copy a file."},
	{Name: "put", Desc: "Write bytes to a file."},
	{Name: "stat", Desc: "Show file metadata."},
	{Name: "ticks", Desc: "Show current kernel tick counter."},
	{Name: "uptime", Desc: "Show uptime (ticks)."},
	{Name: "version", Desc: "Show build version."},
	{Name: "uname", Desc: "Show system information (try -a)."},
	{Name: "panic", Desc: "Panic the shell task (test)."},
	{Name: "log", Desc: "Send a log line to logger service."},
	{Name: "scrollback", Desc: "Show the last N output lines."},
	{Name: "vi", Desc: "Edit a file (SparkVi)."},
	{Name: "rtdemo", Desc: "Start raytracing demo (exit with q/ESC)."},
}
