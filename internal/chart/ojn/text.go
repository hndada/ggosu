package ojn

import (
	"bytes"
	"strings"
	"unicode"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

const commonHangul = "가각간갈감강개거건걸검게겨격견결경계고곡골공과관광괴교구국군굴궁권귀규그극근글금기긴길김까깨꼬꽃꾸꿈끝나낙난날남낭내너널네녀년노놀농누눈뉴느는늘니다단달담당대더덕데도독돌동되두둘드든들등디따딸때떠또뚜뜨라락란람랑래러런럼레려력련렬령례로록론롤료루류르른를름리린림립마막만말맘망매머먹멀메며면명모목몰몸못무문물므미민밀바박반발밤방배백버번벌범법베벨벽변별병보복본볼봄봉부북분불붉비빈빛빠뻐뽀사삭산살삼상새색생서석선설섬성세셔소속손솔송쇼수숙순술숨쉬스슬슴습승시식신실심십싸쌍써쏘쑤씨아악안알암압앙애야약양어억언얼엄업에엔여역연열염엽영예오옥온올옵와완왕외요욕용우운울움웃워원월웨위유육윤율은을음응의이익인일임입있자작잔잠장재저적전절점정제조족존종좋좌죄주죽준줄중즈즐지직진질집짜째쪽차착찬찰참창채처천철청체초촌총최추축춘출춤충취츠층치친칠침카칸칼커컨케코콘콜쿠퀸크큰클키킹타탄탈탐탑태터테텔토톤톱통투트특티틴파판팔패퍼페펜편평포폭표푸풀품풍프플피필하학한할함합항해했행향허헌헤혀혁현혈협형혜호혹혼홀홍화확환활황회효후훈휘휴흐흑흔흘흥희흰히힘" + "둥쌤샘깅꿍냥넷뎌똥뚱띠랩렌롱룸링맨맥멜몽뮤믹밴뱅벳봇뷰블빅빌빙뽕뿐쁘삐샤샵셀셋션셜쉐슈슨싱쌈쎄쏭쓰씬앤앨액얀엘엠옐웅웹윈윙잉잭잼쟁젠젤져졌죠쥬즘찌챔챠첼쳐쵸츄캐캔캠켓켜콩쿨퀘큐킨탱텐템톰툴튜틀팅팝팬팡퍽펀펑펠폰퐁퓨픈픽핑햄헬헨휠힙됐됩됨봐왜줘겠었았갔났왔봤셨없않많같넘뭐뭔왠웬넌렇떻랬렸맞맡묘묻밖받밥벗붙뿌삶섯숲쉽싶쌓씩앉앞얘얻옆옛왼읽잃잊잎젊짓짧찾책춥춰큼탁털텅펴핀햇헐홀흙힌옹녕냐뇨녁닐닙님닥딩뎅랄렛료룰릴립멋몬밍볼뵈뷔삿샹셰솜쉿슝쏙씽얍엉엑옴욱웠윽잘젯좀쥐짐쯤찜츠캡컵켈콤큰킥탭텍팀폴퓰픔핫헉혹훔휙"

const commonHan = "的一是不了人我在有他这中大来上国个到说们为子和你地出道也时年得就那要下以生会自着去之过家学对可她里后小么心多天而能好都然没日于起还发成事只作当想看文无开手十用主行方又如前所本见经头面公同三已老从动两长知民样现分将外但身些与高意进把法此实回二理美点月明其种声全工己话儿者向情部正名定女问力机给等几很业最间新什打便位因重被走电四第门相次东政海口使教西再平真听世气信北少关并内加化由却代军产入先山五太水万市眼体别处总才场师书比住员九笑性通目华报立马命张活难神数件安表原车白应路期叫死常提感金何更反合放做系计或司利受光王果亲界及今京务制解各任至清物台象记边共风战干接它许八特觉望直服毛林题建南度统色字请交爱让认算论百吃义科怎元社术结六功指思非流每青管夫连远资队跟带花快条院变联言权往展该领传近留红治决周保达办运武半候七必城父强步完革深区即求品士转量空甚众技轻程告江语英基派满式李息写呢识极令黄德收脸钱党倒未持取设始版双历越史商千片容研像找友孩站广改议形委早房音火际则首单据导影失拿网香似斯专石若兵弟谁校读志飞观争究包组造落视济喜离虽坏兴切" +
	"歌曲舞梦恋春夏秋冬雨雪星夜翼伤忆永恒幻魔龙凤樱宫雀朱陶杰伦鸡尾酒微祈愿谢燕徐钰怀贤齐杜伟忧旋律创抄鬼泪歌唱乐恋愛風樂華龍鳳電夢戀東暁熱疾斃待黃湘怡翠央宮雲淚憶傷劍舞桜色恋君僕私空海島森姬姫"

// decodeText reads NUL-padded OJN strings in the one encoding that makes the most of them ordinary text:
// O2Jam wrote EUC-KR, while O2Mania packs mix in GBK, Big5 and Shift-JIS.
func decodeText(fields ...[]byte) []string {
	best, bestScore := []string(nil), -1.0
	for i, f := range fields {
		if n := bytes.IndexByte(f, 0); n >= 0 {
			fields[i] = f[:n]
		}
	}
	for _, e := range []encoding.Encoding{korean.EUCKR, simplifiedchinese.GBK, traditionalchinese.Big5, japanese.ShiftJIS} {
		out, good, total, ok := make([]string, len(fields)), 0.0, 0, true
		for i, f := range fields {
			b, err := e.NewDecoder().Bytes(f)
			s := string(b)
			if err != nil || strings.ContainsRune(s, unicode.ReplacementChar) {
				ok = false
				break
			}
			out[i] = strings.TrimSpace(s)
		}
		// Korean titles rarely mix in hanja; Hangul next to Han is a Chinese title read as EUC-KR.
		mixed := strings.ContainsFunc(strings.Join(out, ""), func(r rune) bool { return unicode.Is(unicode.Hangul, r) && r < 0x3130 || r >= 0xAC00 && r <= 0xD7A3 })
		chinese := e == simplifiedchinese.GBK || e == traditionalchinese.Big5
		for _, s := range out {
			runes := []rune(s)
			for i, r := range runes {
				if r < 0x80 {
					continue
				}
				total++
				switch {
				case kana(r):
					// GBK and Big5 also encode kana; a lone one between other characters is usually a misread.
					if chinese && (i == 0 || !kana(runes[i-1])) && (i+1 == len(runes) || !kana(runes[i+1])) {
						good += .5
					} else {
						good++
					}
				case unicode.Is(unicode.Hangul, r):
					if strings.ContainsRune(commonHangul, r) {
						good++
					}
				case unicode.Is(unicode.Han, r):
					if mixed {
					} else if strings.ContainsRune(commonHan, r) {
						good++
					} else {
						good += .3 // a real but rarer character
					}
				case r >= 0xFF61 && r <= 0xFF9F: // half-width katakana: a misread
				case r < 0x250 || unicode.In(r, unicode.Cyrillic, unicode.Greek) || r >= 0x2150 && r <= 0x218F || r >= 0xFE30 && r <= 0xFE6F:
					// Accented Latin, Cyrillic, Greek, roman numerals and small/vertical forms come from
					// double-byte tables read with the wrong encoding.
				default:
					good += .5
				}
			}
		}
		score := 1.0
		if total > 0 {
			score = good / float64(total)
		}
		if ok && score > bestScore {
			best, bestScore = out, score
		}
	}
	if best == nil {
		best = make([]string, len(fields))
		for i, f := range fields {
			b, _ := korean.EUCKR.NewDecoder().Bytes(f)
			best[i] = strings.TrimSpace(string(b))
		}
	}
	return best
}

func kana(r rune) bool { return unicode.In(r, unicode.Hiragana, unicode.Katakana) && r < 0xFF00 }
