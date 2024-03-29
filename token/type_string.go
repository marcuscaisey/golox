// Code generated by "stringer -type Type -linecomment"; DO NOT EDIT.

package token

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[Illegal-0]
	_ = x[EOF-1]
	_ = x[keywordsStart-2]
	_ = x[Print-3]
	_ = x[Var-4]
	_ = x[True-5]
	_ = x[False-6]
	_ = x[Nil-7]
	_ = x[If-8]
	_ = x[Else-9]
	_ = x[And-10]
	_ = x[Or-11]
	_ = x[While-12]
	_ = x[For-13]
	_ = x[Break-14]
	_ = x[Continue-15]
	_ = x[Function-16]
	_ = x[Return-17]
	_ = x[Class-18]
	_ = x[This-19]
	_ = x[Super-20]
	_ = x[keywordsEnd-21]
	_ = x[Ident-22]
	_ = x[String-23]
	_ = x[Number-24]
	_ = x[Semicolon-25]
	_ = x[Comma-26]
	_ = x[Dot-27]
	_ = x[Equal-28]
	_ = x[Plus-29]
	_ = x[Minus-30]
	_ = x[Asterisk-31]
	_ = x[Slash-32]
	_ = x[Percent-33]
	_ = x[Less-34]
	_ = x[LessEqual-35]
	_ = x[Greater-36]
	_ = x[GreaterEqual-37]
	_ = x[EqualEqual-38]
	_ = x[BangEqual-39]
	_ = x[Bang-40]
	_ = x[Question-41]
	_ = x[Colon-42]
	_ = x[LeftParen-43]
	_ = x[RightParen-44]
	_ = x[LeftBrace-45]
	_ = x[RightBrace-46]
}

const _Type_name = "ILLEGALEOFkeywordsStartprintvartruefalsenilifelseandorwhileforbreakcontinuefunreturnclassthissuperkeywordsEndidentifierstringnumber;,.=+-*/%<<=>>===!=!?:(){}"

var _Type_index = [...]uint8{0, 7, 10, 23, 28, 31, 35, 40, 43, 45, 49, 52, 54, 59, 62, 67, 75, 78, 84, 89, 93, 98, 109, 119, 125, 131, 132, 133, 134, 135, 136, 137, 138, 139, 140, 141, 143, 144, 146, 148, 150, 151, 152, 153, 154, 155, 156, 157}

func (i Type) String() string {
	if i >= Type(len(_Type_index)-1) {
		return "Type(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _Type_name[_Type_index[i]:_Type_index[i+1]]
}
