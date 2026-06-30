package optparse

// DefaultOpts is swl2 default_opts (-p, -a, -v).
var DefaultOpts = Optparser(
	Flag("-p", "--passthrough").As("passthrough").Help("let the sink handle the data but still forward it").Group("BASE SWL OPTIONS"),
	Param("-a", "--alias").As("alias").Help("give another name to this component in the pipe").Group("BASE SWL OPTIONS"),
	Flag("-v", "--verbose").As("verbose").Repeat().Help("increase handler verbosity").Group("BASE SWL OPTIONS"),
)

// DefaultColSQLOpts is swl2 default_col_sql_src_opts (table name + optional query).
var DefaultColSQLOpts = Optparser(
	Arg("name").Required().Help("collection/table name"),
	Param("-q", "--query").As("query").Help("SQL query instead of SELECT *"),
)
