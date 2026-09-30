package readme

func githubBase(
	owner string,
	repo string,
	ref string,
) RawBase {
	return RawBase{
		Raw:  "https://raw.githubusercontent.com/" + owner + "/" + repo + "/" + ref + "/{file}",
		Blob: "https://github.com/" + owner + "/" + repo + "/blob/" + ref + "/{file}",
	}
}
