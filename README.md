# clob-trading

포트폴리오용 CLOB(central limit order book) 시뮬레이터. 핵심 아이디어는 **가격대별 FIFO 큐**를 `PriceLevel`로 래핑하고, 레벨을 **Min/Max 힙**에 올려 최우선 호가를 O(1)/O(log N)로 다루는 것.

## 데이터 구조
- `Queue`(container/list): 같은 가격대의 주문을 FIFO로 유지.
- `PriceLevel`: `{Price, Queue, TotalAmount, Index}`로 큐를 감싸고 힙 내 위치(`Index`)를 저장.
- 힙: Bid는 `MaxPriceHeap`, Ask는 `MinPriceHeap`으로 가격 우선순위 유지.
- 인덱스: `OrderBook.Index`에 `orderID → *list.Element`를 저장해 주문 취소/수정 시 O(1) 접근.

## 주문 라이프사이클
- 생성: `CreateOrder` → `AddOrder`가 가격대 없으면 새 레벨을 만들고 힙에 넣은 뒤 큐에 푸시, 인덱스에 기록.
- 취소: `RemoveOrder`가 인덱스로 노드를 찾아 큐에서 제거, `TotalAmount` 차감. 레벨 큐가 비면 힙·맵에서 가격대를 삭제.
- 수정: `EditOrder(EditRequest)`로 처리.
  - 가격 변경: 기존 레벨에서 제거 후 새 가격으로 재삽입(순번 리셋).
  - 수량 증가: 공정성 위해 기존 레벨에서 제거 후 재삽입(뒤로 배치).
  - 수량 감소: 위치 유지, 수량·누적만 조정.

이 구조 덕분에 최우선 호가 조회/매칭은 힙으로 빠르게, 주문 취소·수정은 인덱스로 O(1) 접근해 처리합니다.
