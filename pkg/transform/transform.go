package transform

func Encode(input string) string {
	result := make([]byte, len(input))

	for index := range input {
		result[index] = input[index] + 1
	}

	return string(result)
}

// Optional functionality
// TODO @byrontran: Come back later and write this?
func Decode(input string) string {
	result := make([]byte, len(input))

	for index := range input {
		result[index] = input[index] - 1
	}

	return string(result)
}
