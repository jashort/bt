package internal

//func TestParseTimestamp(t *testing.T) {
//	type args struct {
//		at string
//	}
//	tests := []struct {
//		name    string
//		args    args
//		want    string
//		wantErr bool
//	}{
//		{"Parse", args{"2023-01-01 12:00 PM"}, "2023-01-01 12:00 PM PST", false},
//	}
//	portland, _ := time.LoadLocation("America/Los_Angeles")
//	baseTime := time.Date(2023, 1, 1, 0, 0, 0, 0, portland)
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			got, err := ParseTimestamp(tt.args.at, baseTime)
//			if (err != nil) != tt.wantErr {
//				t.Errorf("ParseTimestamp() error = %v, wantErr %v", err, tt.wantErr)
//				return
//			}
//			if got != tt.want {
//				t.Errorf("ParseTimestamp() got = %v, want %v", got, tt.want)
//			}
//		})
//	}
//}
